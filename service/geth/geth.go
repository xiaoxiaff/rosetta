// Copyright 2020 Celo Org
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package geth

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/kliento/utils/chain"
	"github.com/celo-org/rosetta/internal/fileutils"
	"github.com/celo-org/rosetta/service"
)

type GethOpts struct {
	GethBinary  string
	GenesisPath string
	IpcPath     string
	LogsPath    string
	Datadir     string
	StaticNodes string
	Bootnodes   string
	Verbosity   string
	PublicIp    string
	Cache       string
	RpcAddr     string
	RpcPort     string
	RpcVHosts   string
	SyncMode    string
	GcMode      string
	MaxPeers    string
}

type gethService struct {
	opts *GethOpts

	chainParams *chain.ChainParameters

	cmd     *exec.Cmd
	running service.RunningLock
	logger  log.Logger
}

func NewGethService(opts *GethOpts) *gethService {
	return &gethService{
		opts:   opts,
		logger: log.New("service", "geth"),
	}
}

func (gs *gethService) IpcFilePath() string {
	return gs.opts.IpcFile()
}

func (gs *gethService) ChainParameters() *chain.ChainParameters {
	return gs.chainParams
}

func (gs *gethService) Name() string {
	return "geth"
}

func (gs *gethService) Running() bool {
	return gs.running.Running()
}

func (gs *gethService) Setup() error {
	if err := os.MkdirAll(gs.opts.Datadir, os.ModePerm); err != nil {
		return fmt.Errorf("Can't create celo-blockchain datadir: %w", err)
	}

	if gs.opts.LogsPath != "" {
		if err := os.MkdirAll(filepath.Dir(gs.opts.LogsPath), os.ModePerm); err != nil {
			return fmt.Errorf("Can't create custom logs directory: %w", err)
		}
	}

	// Read Genesis to get chain parameters
	gs.chainParams = chainParamsFromGenesisFile(gs.opts.GenesisPath)

	if err := gs.ensureGethInit(); err != nil {
		return err
	}

	return nil
}

func (gs *gethService) Start(ctx context.Context) error {
	if err := gs.running.EnableOrFail(); err != nil {
		return err
	}
	defer gs.running.Disable()

	if err := gs.Setup(); err != nil {
		return err
	}

	if gs.opts.StaticNodes != "" {
		if err := gs.setupStaticNodes(); err != nil {
			return err
		}
	}

	gethStderr, err := os.OpenFile(gs.opts.LogFile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		gs.logger.Error("Can't open geth logfile", "err", err)
		return err
	}
	defer gethStderr.Close()

	if err := gs.startGeth(gethStderr); err != nil {
		return err
	}

	// Listen to stop signal, and kill the process
	go func() {
		<-ctx.Done()
		if err := gs.cmd.Process.Signal(os.Interrupt); err != nil {
			// Not much else to do. Failed to send a signal
			panic(fmt.Errorf("Error sending signal: %w ", err))
		}
	}()

	if err := gs.cmd.Wait(); err != nil {
		return err
	}

	return nil

}

func (gs *gethService) gethCmd(args ...string) *exec.Cmd {
	datadir := gs.opts.Datadir
	fullArgs := append([]string{"--datadir", datadir}, args...)
	return exec.Command(gs.opts.GethBinary, fullArgs...) //nolint:gosec
}

func (gs *gethService) setupStaticNodes() error {
	var staticNodesRaw []byte
	var err error

	staticNodes := strings.Split(gs.opts.StaticNodes, ",")
	if staticNodesRaw, err = json.Marshal(staticNodes); err != nil {
		return fmt.Errorf("Can't serialize static nodes: %w", err)
	}
	//nolint:gosec
	if err = ioutil.WriteFile(gs.opts.StaticNodesFile(), staticNodesRaw, 0644); err != nil {
		return fmt.Errorf("Can't serialize static nodes: %w", err)
	}

	return nil
}

func (gs *gethService) ensureGethInit() error {
	// Check if geth is initialized already
	flagFile := gs.opts.GethInitializedFile()

	if fileutils.FileExists(flagFile) {
		gs.logger.Info("Geth Already initialized... skipping init")
		return nil
	}

	gs.logger.Info("Running geth init")
	out, err := gs.gethCmd("init", gs.opts.GenesisPath).CombinedOutput()
	if err != nil {
		gs.logger.Error("Error running geth init", "err", err)
		fmt.Println(string(out))
		return err
	}
	if err = fileutils.TouchFile(flagFile); err != nil {
		gs.logger.Error("Error creating marker file", "err", err)
		return err
	}
	return nil
}

func (gs *gethService) startGeth(stdErr *os.File) error {
	gethArgs := []string{
		"--nousb",
		"--rpc",
		"--rpcaddr", gs.opts.RpcAddr,
		"--rpcport", gs.opts.RpcPort,
		"--rpcvhosts", gs.opts.RpcVHosts,
		"--syncmode", gs.opts.SyncMode,
		"--gcmode", gs.opts.GcMode,
		"--rpcapi", "eth,net,web3,debug,admin,personal",
		"--ipcpath", gs.IpcFilePath(),
		"--light.serve", "0",
		"--light.maxpeers", "0",
		"--maxpeers", gs.opts.MaxPeers,
		"--consoleformat", "term",
		// "--consoleoutput", "split",
	}

	// Fix necessary flag for node to hardfork properly (temp fix)
	switch gs.chainParams.ChainId.String() {
	case params.AlfajoresChainConfig.ChainID.String():
		gethArgs = append([]string{"--alfajores"}, gethArgs...)
	case params.BaklavaChainConfig.ChainID.String():
		gethArgs = append([]string{"--baklava"}, gethArgs...)
	case params.MainnetChainConfig.ChainID.String():
		break
	default:
		gethArgs = append(gethArgs, "--networkid", gs.chainParams.ChainId.String())
	}

	if gs.opts.Verbosity != "" {
		gethArgs = append(gethArgs, "--verbosity", gs.opts.Verbosity)
	}

	if gs.opts.Bootnodes != "" {
		gethArgs = append(gethArgs, "--bootnodes", gs.opts.Bootnodes)
	}

	if gs.opts.PublicIp != "" {
		gethArgs = append(gethArgs, "--nat", "extip:"+gs.opts.PublicIp)
	}

	if gs.opts.Cache != "" {
		gethArgs = append(gethArgs, "--cache", gs.opts.Cache)
	}

	fmt.Println("geth", strings.Join(gethArgs, " "))

	cmd := gs.gethCmd(gethArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stderr = stdErr

	if err := cmd.Start(); err != nil {
		gs.logger.Error("Error starting geth", "err", err)
		return err
	}

	gs.cmd = cmd
	return nil
}

func (gopts GethOpts) GethInitializedFile() string {
	return filepath.Join(gopts.Datadir, ".geth-initialized")
}

func (gopts GethOpts) LogFile() string {
	if gopts.LogsPath == "" {
		return filepath.Join(gopts.Datadir, "celo.log")
	}
	return gopts.LogsPath
}

func (gopts GethOpts) IpcFile() string {
	if gopts.IpcPath == "" {
		return filepath.Join(gopts.Datadir, "geth.ipc")
	}
	return gopts.IpcPath
}

func (gopts GethOpts) StaticNodesFile() string {
	return filepath.Join(gopts.Datadir, "/celo/static-nodes.json")
}

func chainParamsFromGenesisFile(genesisPath string) *chain.ChainParameters {
	data, err := ioutil.ReadFile(genesisPath)
	if err != nil {
		log.Crit("Can't read genesis.json on DataDir", "genesisPath", genesisPath, "err", err)
	}

	// We only map the fields we need
	var genesis struct {
		Config struct {
			ChainId  uint64 `json:"chainId"`
			Isntabul struct {
				Epoch uint64 `json:"epoch"`
			} `json:"istanbul"`
		} `json:"config"`
	}

	if err = json.Unmarshal(data, &genesis); err != nil {
		log.Crit("Can't parse genesis.json on DataDir", "genesisPath", genesisPath, "err", err)
	}

	return &chain.ChainParameters{
		ChainId:   new(big.Int).SetUint64(genesis.Config.ChainId),
		EpochSize: genesis.Config.Isntabul.Epoch,
	}
}
