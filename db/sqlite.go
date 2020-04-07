package db

import (
	"context"
	"database/sql"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	_ "github.com/mattn/go-sqlite3"
)

type sqlRosettaDb struct {
	db *sql.DB
}

const (
	setLastPersistedBlockStmt = `
	case when exists select lastPersistedBlock from stats 
			then update stats set lastPersistedBlock = ?
			else insert into stats (lastPersistedBlock) values (?)
	`
	setGasPriceMinimumOnStmt   = "insert into gasPriceMinimum (fromBlock, val) values (?, ?, ?)"
	setRegisteredAddressOnStmt = "insert into registeredAddresses (contract, fromBlock, fromTx, address) values (?, ?, ?, ?)"
)

func NewSQLDB() (*sqlRosettaDb, error) {
	os.Remove("./monitor.db")
	db, err := sql.Open("sqlite3", "./monitor.db")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("CREATE TABLE registryAddresses (contract chars(32), fromBlock bigint, fromTx int, address chars(40))"); err != nil {
		return nil, err
	}
	if _, err := db.Exec("CREATE TABLE gasPriceMinimum (fromBlock bigint, val bigint)"); err != nil {
		return nil, err
	}
	if _, err := db.Exec("CREATE TABLE stats (lastPersistedBlock bigint)"); err != nil { //TODO: limit entries to 1?
		return nil, err
	}
	return &sqlRosettaDb{
		db: db,
	}, nil
}

func (cs *sqlRosettaDb) LastPersistedBlock(ctx context.Context) (*big.Int, error) {
	rows, err := cs.db.Query("select lastPersistedBlock from stats")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var block *big.Int
	if rows.Next() {
		if err := rows.Scan(block); err != nil {
			return nil, err
		}
		log.Info("Last Persisted Block Found", "block", block.Uint64())
	} else {
		return nil, rows.Err()
	}

	return block, nil
}

func (cs *sqlRosettaDb) GasPriceMinimunOn(ctx context.Context, block *big.Int) (*big.Int, error) {
	rows, err := cs.db.Query("select val from gasPriceMinimum where fromBlock <= ? order by desc fromblock limit 1", block)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var value *big.Int
	if rows.Next() {
		if err := rows.Scan(value); err != nil {
			return nil, err
		}
		log.Info("Gas Price Minimum Found", "block", block.Uint64(), "val", value.Uint64())
	} else {
		return nil, rows.Err()
	}

	return value, nil
}

func (cs *sqlRosettaDb) RegistryAddressOn(ctx context.Context, block *big.Int, txIndex uint, contractName string) (common.Address, error) {
	rows, err := cs.db.Query("select address from registryAddresses where id == ? and fromBlock <= ? and fromTx <= ? order by desc fromblock, fromTx limit 1", contractName, block, txIndex)
	if err != nil {
		return common.ZeroAddress, err
	}
	defer rows.Close()

	var address common.Address
	if rows.Next() {
		if err := rows.Scan(&address); err != nil {
			return common.ZeroAddress, err
		}
		log.Info("Registry Address Found", "contract", contractName, "address", address)
	} else {
		return common.ZeroAddress, rows.Err()
	}

	return address, nil
}

func (cs *sqlRosettaDb) RegistryAddressesOn(ctx context.Context, block *big.Int, txIndex uint, contractNames ...string) (map[string]common.Address, error) {
	addresses := make(map[string]common.Address)
	// TODO: Could this be done more efficiently, perhaps concurrently?
	for _, name := range contractNames {
		address, err := cs.RegistryAddressOn(ctx, block, txIndex, name)
		if err != nil {
			return nil, err
		}
		addresses[name] = address
	}
	return addresses, nil
}

func (cs *sqlRosettaDb) ApplyChanges(ctx context.Context, changeSet *BlockChangeSet) error {

	//TODO: check if this is the right isolation level, or should keep default
	tx, err := cs.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, setLastPersistedBlockStmt, changeSet.BlockNumber, changeSet.BlockNumber); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return rollbackErr
		}
		return err
	}
	if changeSet.GasPriceMinimun != nil {
		if _, err := tx.ExecContext(ctx, setGasPriceMinimumOnStmt, changeSet.BlockNumber, changeSet.GasPriceMinimun); err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				return rollbackErr
			}
			return err
		}
	}
	for _, rc := range changeSet.RegistryChanges {
		if _, err := tx.ExecContext(ctx, setRegisteredAddressOnStmt, rc.Contract, changeSet.BlockNumber, rc.TxIndex, rc.NewAddress); err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				return rollbackErr
			}
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func (cs *sqlRosettaDb) setLastPersistedBlock(block *big.Int) error {
	_, err := cs.db.Exec(setLastPersistedBlockStmt, block, block)
	if err != nil {
		return err
	}
	return nil
}

func (cs *sqlRosettaDb) setGasPriceMinimumOn(block, gasPriceMinimum *big.Int) error {
	_, err := cs.db.Exec(setGasPriceMinimumOnStmt, block, gasPriceMinimum)
	if err != nil {
		return err
	}
	return nil
}

func (cs *sqlRosettaDb) setRegisteredAddressOn(contractName string, blockNumber *big.Int, txIndex uint, address common.Address) error {
	_, err := cs.db.Exec(setRegisteredAddressOnStmt, contractName, blockNumber, txIndex, address)
	if err != nil {
		return err
	}
	return nil
}