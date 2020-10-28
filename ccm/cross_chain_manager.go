/*
 * Copyright (C) 2020 The poly network Authors
 * This file is part of The poly network library.
 *
 * The  poly network  is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Lesser General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * The  poly network  is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Lesser General Public License for more details.
 * You should have received a copy of the GNU Lesser General Public License
 * along with The poly network .  If not, see <http://www.gnu.org/licenses/>.
 */
package ccm

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	common2 "github.com/ethereum/go-ethereum/common"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/polynetwork/fabric-contract/utils"
	"github.com/polynetwork/poly/common"
	"github.com/polynetwork/poly/consensus/vbft/config"
	"github.com/polynetwork/poly/core/types"
	"github.com/polynetwork/poly/merkle"
	common3 "github.com/polynetwork/poly/native/service/cross_chain_manager/common"
	"github.com/polynetwork/poly/native/service/header_sync/ont"
	"math/big"
	"strconv"
)

const (
	FabricChainID             = "poly_fabric_chain_id"
	PolyConsensusPeersKey     = "poly_consensus_peers"
	PolyGenesisHeader         = "poly_genesis_header"
	CrossChainManagerDeployer = "ccmdepolyer"
	PolyEpochHeight           = "poly_epoch_height"
	CrossChainId              = "poly_cross_chain_id"
	ToPolyTx                  = "to_poly"
	FromPolyTx                = "from_poly"
)

type CrossChainManager struct{}

func (manager *CrossChainManager) Init(stub shim.ChaincodeStubInterface) peer.Response {
	args := stub.GetArgs()
	if len(args) != 1 {
		return shim.Error("wrong length of args")
	}
	chainId, err := strconv.ParseUint(string(args[0]), 10, 64)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to parse chainId: %v", err))
	}
	rawChainId := make([]byte, 8)
	binary.LittleEndian.PutUint64(rawChainId, chainId)
	if err := stub.PutState(FabricChainID, rawChainId); err != nil {
		return shim.Error(fmt.Sprintf("failed to put deployer: %v", err))
	}

	op, err := utils.GetMsgSenderAddress(stub)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to get tx sender: %v", err))
	}
	if err = stub.PutState(CrossChainManagerDeployer, op.Bytes()); err != nil {
		return shim.Error(fmt.Sprintf("failed to put deployer: %v", err))
	}
	zero := big.NewInt(0)
	if err = stub.PutState(CrossChainId, zero.Bytes()); err != nil {
		return shim.Error(fmt.Sprintf("failed to put cross chain id zero: %v", err))
	}
	return shim.Success(nil)
}

func (manager *CrossChainManager) Invoke(stub shim.ChaincodeStubInterface) peer.Response {
	function, _ := stub.GetFunctionAndParameters()
	args := stub.GetArgs()
	if len(args) == 0 {
		return shim.Error("no args")
	}
	args = args[1:]

	switch function {
	case "initGenesisBlock":
		return manager.initGenesisBlock(stub, args)
	case "changeBookKeeper":
		return manager.changeBookKeeper(stub, args)
	case "crossChain":
		return manager.crossChain(stub, args)
	case "verifyHeaderAndExecuteTx":
		return manager.verifyHeaderAndExecuteTx(stub, args)
	}

	return shim.Error("Invalid invoke function name. Expecting \"initGenesisBlock\" \"changeBookKeeper\" \"crossChain\" \"verifyHeaderAndExecuteTx\"")
}

func (manager *CrossChainManager) initGenesisBlock(stub shim.ChaincodeStubInterface, args [][]byte) peer.Response {
	if len(args) != 1 {
		return shim.Error(fmt.Sprintf("wrong number of args: get %d but 1 expected", len(args)))
	}

	sender, err := utils.GetMsgSenderAddress(stub)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to get tx sender: %v", err))
	}
	rawDeployer, err := stub.GetState(CrossChainManagerDeployer)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to get deployer: %v", err))
	}
	if !bytes.Equal(rawDeployer, sender.Bytes()) {
		return shim.Error(fmt.Sprintf("only deployer can call this function"))
	}

	raw, _ := stub.GetState(PolyConsensusPeersKey)
	if raw != nil {
		return shim.Error("genesis info already init")
	}
	rawHdr, err := hex.DecodeString(string(args[0]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode hex genesis header: %v", err))
	}
	hdr := &types.Header{}
	if err := hdr.Deserialization(common.NewZeroCopySource(rawHdr)); err != nil {
		return shim.Error(fmt.Sprintf("failed to deserialize genesis header: %v", err))
	}
	if err := stub.PutState(PolyGenesisHeader, rawHdr); err != nil {
		return shim.Error(fmt.Sprintf("failed to put raw genesis header: %v", err))
	}
	blkInfo := &vconfig.VbftBlockInfo{}
	if err := json.Unmarshal(hdr.ConsensusPayload, blkInfo); err != nil {
		return shim.Error(fmt.Sprintf("unmarshal VbftBlockInfo error: %v", err))
	}
	if blkInfo.NewChainConfig == nil {
		return shim.Error("no NewChainConfig in VbftBlockInfo")
	}
	consensusPeers := &ont.ConsensusPeers{
		ChainID: hdr.ChainID,
		Height:  hdr.Height,
		PeerMap: make(map[string]*ont.Peer),
	}
	for _, p := range blkInfo.NewChainConfig.Peers {
		consensusPeers.PeerMap[p.ID] = &ont.Peer{Index: p.Index, PeerPubkey: p.ID}
	}
	sink := common.NewZeroCopySink(nil)
	consensusPeers.Serialization(sink)
	if err := stub.PutState(PolyConsensusPeersKey, sink.Bytes()); err != nil {
		return shim.Error(fmt.Sprintf("put ConsensusPeer error: %v", err))
	}
	rawHeight := make([]byte, 4)
	binary.LittleEndian.PutUint32(rawHeight, hdr.Height)
	if err := stub.PutState(PolyEpochHeight, rawHeight); err != nil {
		return shim.Error(fmt.Sprintf("failed to save epoch height: %v", err))
	}

	return shim.Success(nil)
}

func (manager *CrossChainManager) changeBookKeeper(stub shim.ChaincodeStubInterface, args [][]byte) peer.Response {
	if len(args) != 1 {
		return shim.Error(fmt.Sprintf("wrong number of args: get %d but 1 expected", len(args)))
	}

	raw, _ := stub.GetState(PolyConsensusPeersKey)
	if len(raw) == 0 {
		return shim.Error("genesis info not init")
	}
	peers := &ont.ConsensusPeers{}
	if err := peers.Deserialization(common.NewZeroCopySource(raw)); err != nil {
		return shim.Error(fmt.Sprintf("deserialize consensus peers: %v", err))
	}
	rawHdr, err := hex.DecodeString(string(args[0]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode hex header: %v", err))
	}
	hdr := &types.Header{}
	if err := hdr.Deserialization(common.NewZeroCopySource(rawHdr)); err != nil {
		return shim.Error(fmt.Sprintf("failed to deserialize genesis header: %v", err))
	}

	rawEpoch, err := stub.GetState(PolyEpochHeight)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to get the epoch height: %v", err))
	}
	epochHeight := binary.LittleEndian.Uint32(rawEpoch)
	if hdr.Height <= epochHeight {
		return shim.Error(fmt.Sprintf("no need to update book keepers: "+
			"height in state is %d, and your commit is %d", epochHeight, hdr.Height))
	}

	if err := VerifyPolyHeader(hdr, peers); err != nil {
		return shim.Error(fmt.Sprintf("failed to verify header: %v", err))
	}
	blkInfo := &vconfig.VbftBlockInfo{}
	if err := json.Unmarshal(hdr.ConsensusPayload, blkInfo); err != nil {
		return shim.Error(fmt.Sprintf("unmarshal VbftBlockInfo error: %v", err))
	}
	if blkInfo.NewChainConfig == nil {
		return shim.Error("no NewChainConfig in VbftBlockInfo")
	}
	newPeers := &ont.ConsensusPeers{
		ChainID: hdr.ChainID,
		Height:  hdr.Height,
		PeerMap: make(map[string]*ont.Peer),
	}
	for _, p := range blkInfo.NewChainConfig.Peers {
		newPeers.PeerMap[p.ID] = &ont.Peer{Index: p.Index, PeerPubkey: p.ID}
	}

	sink := common.NewZeroCopySink(nil)
	newPeers.Serialization(sink)
	if err := stub.PutState(PolyConsensusPeersKey, sink.Bytes()); err != nil {
		return shim.Error(fmt.Sprintf("put ConsensusPeer error: %v", err))
	}
	rawHeight := make([]byte, 4)
	binary.LittleEndian.PutUint32(rawHeight, hdr.Height)
	if err := stub.PutState(PolyEpochHeight, rawHeight); err != nil {
		return shim.Error(fmt.Sprintf("failed to save epoch height: %v", err))
	}

	return shim.Success(nil)
}

func (manager *CrossChainManager) crossChain(stub shim.ChaincodeStubInterface, args [][]byte) peer.Response {
	if len(args) != 5 {
		return shim.Error(fmt.Sprintf("wrong number of args: get %d but 5 expected", len(args)))
	}

	rawCcid, err := stub.GetState(CrossChainId)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to get raw cross chain id: %v", err))
	}
	ccid := big.NewInt(0).SetBytes(rawCcid)
	rawCcid = common2.BytesToHash(rawCcid).Bytes()

	rawTxid, err := hex.DecodeString(stub.GetTxID())
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode txid: %v", err))
	}

	toChainId, err := strconv.ParseUint(string(args[0]), 10, 64)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to parse tochainId: %v", err))
	}
	toContract, err := hex.DecodeString(string(args[1]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode toContract: %v", err))
	}
	rawArgs, err := hex.DecodeString(string(args[3]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode args: %v", err))
	}

	res := &common3.MakeTxParam{
		TxHash:              rawTxid,
		Method:              string(args[2]),
		CrossChainID:        rawCcid,
		FromContractAddress: args[4],
		ToContractAddress:   toContract,
		ToChainID:           toChainId,
		Args:                rawArgs,
	}
	sink := common.NewZeroCopySink(nil)
	res.Serialization(sink)
	raw := sink.Bytes()

	key := fmt.Sprintf("%s-%s", ToPolyTx, hex.EncodeToString(rawCcid))
	if err := stub.PutState(key, raw); err != nil {
		return shim.Error(fmt.Sprintf("failed to save this cross chain info: %v", err))
	}

	ccid = ccid.Add(ccid, big.NewInt(1))
	if err = stub.PutState(CrossChainId, ccid.Bytes()); err != nil {
		return shim.Error(fmt.Sprintf("failed to put cross chain id: %v", err))
	}

	if err := stub.SetEvent(key, raw); err != nil {
		return shim.Error(fmt.Sprintf("failed to set event: %v", err))
	}

	return shim.Success(nil)
}

func (manager *CrossChainManager) verifyHeaderAndExecuteTx(stub shim.ChaincodeStubInterface, args [][]byte) peer.Response {
	if len(args) != 4 {
		return shim.Error(fmt.Sprintf("wrong number of args: get %d but 1 expected", len(args)))
	}

	raw, _ := stub.GetState(PolyConsensusPeersKey)
	if len(raw) == 0 {
		return shim.Error("genesis info not init")
	}
	peers := &ont.ConsensusPeers{}
	if err := peers.Deserialization(common.NewZeroCopySource(raw)); err != nil {
		return shim.Error(fmt.Sprintf("deserialize consensus peers: %v", err))
	}

	rawEpoch, err := stub.GetState(PolyEpochHeight)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to get the epoch height: %v", err))
	}
	epochHeight := binary.LittleEndian.Uint32(rawEpoch)

	rawHdr, err := hex.DecodeString(string(args[1]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode hex header: %v", err))
	}
	hdr := &types.Header{}
	if err := hdr.Deserialization(common.NewZeroCopySource(rawHdr)); err != nil {
		return shim.Error(fmt.Sprintf("failed to deserialize raw header: %v", err))
	}
	if hdr.Height >= epochHeight {
		if err := VerifyPolyHeader(hdr, peers); err != nil {
			return shim.Error(fmt.Sprintf("failed to verify header: %v", err))
		}
	} else {
		rawAHdr, err := hex.DecodeString(string(args[3]))
		if err != nil {
			return shim.Error(fmt.Sprintf("failed to decode hex header: %v", err))
		}
		anchorHdr := &types.Header{}
		if err := anchorHdr.Deserialization(common.NewZeroCopySource(rawAHdr)); err != nil {
			return shim.Error(fmt.Sprintf("failed to deserialize anchor header: %v", err))
		}
		if err := VerifyPolyHeader(anchorHdr, peers); err != nil {
			return shim.Error(fmt.Sprintf("failed to verify anchor header: %v", err))
		}
		rawHdrProof, err := hex.DecodeString(string(args[2]))
		if err != nil {
			return shim.Error(fmt.Sprintf("failed to decode hex header: %v", err))
		}
		blkHash, err := merkle.MerkleProve(rawHdrProof, anchorHdr.BlockRoot.ToArray())
		if err != nil {
			return shim.Error(fmt.Sprintf("failed to check the merkle proof: %v", err))
		}
		hash := hdr.Hash()
		if !bytes.Equal(hash.ToArray(), blkHash) {
			return shim.Error(fmt.Sprintf("block hash from header-proof not equal"))
		}
	}

	rawProof, err := hex.DecodeString(string(args[0]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode hex header: %v", err))
	}
	val, err := merkle.MerkleProve(rawProof, hdr.CrossStateRoot.ToArray())
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to check the merkle proof: %v", err))
	}
	merkleValue := new(common3.ToMerkleValue)
	if err := merkleValue.Deserialization(common.NewZeroCopySource(val)); err != nil {
		return shim.Error(fmt.Sprintf("deserialize merkleValue error: %v", err))
	}

	rawPolyChainId := make([]byte, 8)
	binary.LittleEndian.PutUint64(rawPolyChainId, hdr.ChainID)
	rawTxId := append(rawPolyChainId, merkleValue.MakeTxParam.CrossChainID...)

	key := fmt.Sprintf("%s-%s", FromPolyTx, hex.EncodeToString(rawTxId))

	if val, _ := stub.GetState(key); len(val) != 0 {
		return shim.Error(fmt.Sprintf("this cross chain tx %s already done", key))
	}
	if err := stub.PutState(key, rawTxId); err != nil {
		return shim.Error(fmt.Sprintf("put key: %s error: %v", key, err))
	}

	rawCid, err := stub.GetState(FabricChainID)
	if err != nil {
		return shim.Error("failed to get chain id of this channel")
	}
	cid := binary.LittleEndian.Uint64(rawCid)
	if cid != merkleValue.MakeTxParam.ToChainID {
		return shim.Error(fmt.Sprintf("target chain id is %d not %d of this channel",
			merkleValue.MakeTxParam.ToChainID, cid))
	}

	if err := stub.SetEvent(key, val); err != nil {
		return shim.Error(fmt.Sprintf("failed to set event %s: %v", key, err))
	}

	invokeArgs := make([][]byte, 2)
	invokeArgs[0] = []byte(merkleValue.MakeTxParam.Method)
	invokeArgs[1] = []byte(hex.EncodeToString(merkleValue.MakeTxParam.Args))
	return stub.InvokeChaincode(string(merkleValue.MakeTxParam.ToContractAddress), invokeArgs, stub.GetChannelID())
}
