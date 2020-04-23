// Code generated by github.com/fjl/gencodec. DO NOT EDIT.

package rpc

import (
	"encoding/json"
	"errors"
	"math/big"
)

// MarshalJSON marshals as JSON.
func (t TransferMetadata) MarshalJSON() ([]byte, error) {
	type TransferMetadata struct {
		Balance *big.Int             `json:"balance" gencodec:"required"`
		Tx      *TransactionMetadata `json:"tx"`
	}
	var enc TransferMetadata
	enc.Balance = t.Balance
	enc.Tx = t.Tx
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (t *TransferMetadata) UnmarshalJSON(input []byte) error {
	type TransferMetadata struct {
		Balance *big.Int             `json:"balance" gencodec:"required"`
		Tx      *TransactionMetadata `json:"tx"`
	}
	var dec TransferMetadata
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.Balance == nil {
		return errors.New("missing required field 'balance' for TransferMetadata")
	}
	t.Balance = dec.Balance
	if dec.Tx != nil {
		t.Tx = dec.Tx
	}
	return nil
}