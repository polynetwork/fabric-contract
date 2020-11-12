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
package lockproxy

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/polynetwork/fabric-contract/utils"
	pcommon "github.com/polynetwork/poly/common"
	"io"
	"math/big"
	"strconv"
)

const (
	ProxyOwner             = "proxy_owner"
	ProxyCCM               = "proxy_ccm"
	ProxyBindKey           = "proxy-%d"
	AssetBindKey           = "asset-%d-%s"
	LockProxyAddr          = "lockproxy_addr"
	FromCCM                = "from_ccm"
	ProxyOwnershipTransfer = "proxy_owner_transfer"
	ProxyTransfer          = "proxyTransfer"
)

var logger = shim.NewLogger("LockProxy")

type TxArgs struct {
	ToAssetHash []byte
	ToAddress   []byte
	Amount      *big.Int
}

func (args *TxArgs) Serialization(sink *pcommon.ZeroCopySink) {
	sink.WriteVarBytes(args.ToAssetHash)
	sink.WriteVarBytes(args.ToAddress)
	raw, _ := PadFixedBytes(args.Amount, 32)
	sink.WriteBytes(raw)
}

func (args *TxArgs) Deserialization(source *pcommon.ZeroCopySource) error {
	assetHash, eof := source.NextVarBytes()
	if eof {
		return fmt.Errorf("Args.Deserialization NextVarBytes AssetHash error:%s", io.ErrUnexpectedEOF)
	}

	toAddress, eof := source.NextVarBytes()
	if eof {
		return fmt.Errorf("Args.Deserialization NextVarBytes ToAddress error:%s", io.ErrUnexpectedEOF)
	}

	value, eof := source.NextBytes(32)
	if eof {
		return fmt.Errorf("Args.Deserialization NextBytes Value error:%s", io.ErrUnexpectedEOF)
	}

	amt, err := UnpadFixedBytes(value, 32)
	if err != nil {
		return fmt.Errorf("faield to get amount: %v", err)
	}

	args.ToAssetHash = assetHash
	args.ToAddress = toAddress
	args.Amount = amt
	return nil
}

type LockProxy struct{}

func (lp *LockProxy) Init(stub shim.ChaincodeStubInterface) pb.Response {
	rawName, _ := stub.GetState(LockProxyAddr)
	if len(rawName) != 0 {
		return shim.Success(nil)
	}

	args := stub.GetStringArgs()
	if len(args) != 0 {
		return shim.Error("wrong args number and should be zero")
	}

	owner, err := utils.GetMsgSenderAddress(stub)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed To get tx sender: %v", err))
	}
	if err = stub.PutState(ProxyOwner, owner.Bytes()); err != nil {
		return shim.Error(fmt.Sprintf("failed To put token owner: %v", err))
	}
	lpAddr := utils.GetAddrFromRaw(append([]byte(LockProxyAddr), owner.Bytes()...))
	if err := stub.PutState(LockProxyAddr, lpAddr.Bytes()); err != nil {
		return shim.Error(fmt.Sprintf("failed to put lockproxy addr: %v", err))
	}

	return shim.Success(nil)
}

func (lp *LockProxy) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	fn, _ := stub.GetFunctionAndParameters()
	args := stub.GetArgs()
	if len(args) == 0 {
		return shim.Error("no args")
	}
	args = args[1:]

	switch fn {
	case "getOwner":
		return lp.getOwner(stub)
	case "getLockProxyAddr":
		return lp.getLockProxyAddr(stub)
	case "transferOwnership":
		return lp.transferOwnership(stub, args)
	case "setManager":
		return lp.setManager(stub, args)
	case "bindProxyHash":
		return lp.bindProxyHash(stub, args)
	case "getProxyHash":
		return lp.getProxyHash(stub, args)
	case "bindAssetHash":
		return lp.bindAssetHash(stub, args)
	case "getAssetHash":
		return lp.getAssetHash(stub, args)
	case "lock":
		return lp.lock(stub, args)
	case "unlock":
		return lp.unlock(stub, args)
	case "getManager":
		return lp.getManager(stub)
	}

	return shim.Error(fmt.Sprintf("no function name %s found", fn))
}

func (lp *LockProxy) getOwner(stub shim.ChaincodeStubInterface) pb.Response {
	owner, err := stub.GetState(ProxyOwner)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(owner)
}

func (lp *LockProxy) getLockProxyAddr(stub shim.ChaincodeStubInterface) pb.Response {
	lpAddr, err := stub.GetState(LockProxyAddr)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(lpAddr)
}

func (lp *LockProxy) transferOwnership(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 1 {
		return shim.Error("number of args should be 1")
	}
	old, err := checkOwner(stub)
	if err != nil {
		return shim.Error(err.Error())
	}
	rawAcc, err := hex.DecodeString(string(args[0]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode hex account: %v", err))
	}
	if err := stub.PutState(ProxyOwner, rawAcc); err != nil {
		return shim.Error(err.Error())
	}
	rawEvent, err := json.Marshal(&TransferOwnershipEvent{
		NewOwner: rawAcc,
		OldOwner: old,
	})
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to json marshal: %v", err))
	}
	if err := stub.SetEvent(ProxyOwnershipTransfer, rawEvent); err != nil {
		return shim.Error(fmt.Sprintf("failed to set event: %v", err))
	}
	return shim.Success(nil)
}

func (lp *LockProxy) setManager(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 1 {
		return shim.Error("args number should be 1")
	}
	if _, err := checkOwner(stub); err != nil {
		return shim.Error(fmt.Sprintf("failed to check owner: %v", err))
	}
	if err := stub.PutState(ProxyCCM, args[0]); err != nil {
		return shim.Error(fmt.Sprintf("failed to put cross chain manager name: %v", err))
	}
	return shim.Success(nil)
}

func (lp *LockProxy) getManager(stub shim.ChaincodeStubInterface) pb.Response {
	raw, err := stub.GetState(ProxyCCM)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to get cross chain manager name: %v", err))
	}
	return shim.Success(raw)
}

func (lp *LockProxy) bindProxyHash(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 2 {
		return shim.Error("args number should be 2")
	}
	if _, err := checkOwner(stub); err != nil {
		return shim.Error(fmt.Sprintf("failed to check owner: %v", err))
	}
	chainId, err := strconv.ParseUint(string(args[0]), 10, 64)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to parse chainId: %v", err))
	}
	target, err := hex.DecodeString(string(args[1]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode hex target proxy: %v", err))
	}
	if err := stub.PutState(getProxyBindKey(chainId), target); err != nil {
		return shim.Error(fmt.Sprintf("failed to put proxy: %v", err))
	}
	return shim.Success(nil)
}

func (lp *LockProxy) bindAssetHash(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 3 {
		return shim.Error("args number should be 3")
	}
	if _, err := checkOwner(stub); err != nil {
		return shim.Error(fmt.Sprintf("failed to check owner: %v", err))
	}
	chainId, err := strconv.ParseUint(string(args[1]), 10, 64)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to parse chainId: %v", err))
	}
	target, err := hex.DecodeString(string(args[2]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode hex target asset: %v", err))
	}
	if err := stub.PutState(getAssetBindKey(chainId, string(args[0])), target); err != nil {
		return shim.Error(fmt.Sprintf("failed to put asset: %v", err))
	}
	return shim.Success(nil)
}

func (lp *LockProxy) getProxyHash(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 1 {
		return shim.Error("args number should be 1")
	}
	chainId, err := strconv.ParseUint(string(args[0]), 10, 64)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to parse chainId: %v", err))
	}
	val, err := stub.GetState(getProxyBindKey(chainId))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to put proxy: %v", err))
	}
	return shim.Success(val)
}

func (lp *LockProxy) getAssetHash(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 2 {
		return shim.Error("args number should be 2")
	}
	chainId, err := strconv.ParseUint(string(args[1]), 10, 64)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to parse chainId: %v", err))
	}
	val, err := stub.GetState(getAssetBindKey(chainId, string(args[0])))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to put asset: %v", err))
	}
	return shim.Success(val)
}

func (lp *LockProxy) lock(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 4 {
		return shim.Error("args number should be 4")
	}
	token := string(args[0])
	if token == "" {
		return shim.Error("token chaincode name is required")
	}

	lpAddr, err := stub.GetState(LockProxyAddr)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to get LockProxyAddr: %v", err))
	}

	from, err := utils.GetMsgSenderAddress(stub)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to get tx sender: %v", err))
	}
	amt, ok := big.NewInt(0).SetString(string(args[3]), 10)
	if !ok {
		return shim.Error(fmt.Sprintf("failed to decode amount: %s", args[2]))
	}
	if amt.Sign() != 1 {
		return shim.Error(fmt.Sprintf("amount should be positive"))
	}

	transferArgs := make([][]byte, 4)
	transferArgs[0] = []byte(ProxyTransfer)
	transferArgs[1] = from.Bytes()
	transferArgs[2] = lpAddr
	transferArgs[3] = amt.Bytes()
	resp := stub.InvokeChaincode(token, transferArgs, "")
	if resp.Status != shim.OK {
		return shim.Error(fmt.Sprintf("failed to lock asset: %s", resp.Message))
	}

	chainId, err := strconv.ParseUint(string(args[1]), 10, 64)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to parse chainId: %v", err))
	}
	toAsset, err := stub.GetState(getAssetBindKey(chainId, token))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to get toAsset: %v", err))
	}
	if len(toAsset) == 0 {
		return shim.Error("get no toAsset")
	}
	toProxy, err := stub.GetState(getProxyBindKey(chainId))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to get toProxy: %v", err))
	}
	if len(toProxy) == 0 {
		return shim.Error("get no toProxy")
	}

	ccm, err := stub.GetState(ProxyCCM)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to get ccm: %v", err))
	}
	if len(ccm) == 0 {
		return shim.Error("get no ccm")
	}

	toAddr, err := hex.DecodeString(string(args[2]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode hex toAddr: %v", err))
	}
	txArgs := &TxArgs{
		ToAssetHash: toAsset,
		ToAddress:   toAddr,
		Amount:      amt,
	}
	sink := pcommon.NewZeroCopySink(nil)
	txArgs.Serialization(sink)

	invokeArgs := make([][]byte, 5)
	invokeArgs[0] = []byte("crossChain")
	invokeArgs[1] = args[1]
	invokeArgs[2] = []byte(hex.EncodeToString(toProxy))
	invokeArgs[3] = []byte("unlock")
	invokeArgs[4] = []byte(hex.EncodeToString(sink.Bytes()))

	resp = stub.InvokeChaincode(string(ccm), invokeArgs, "")
	if resp.Status != shim.OK {
		return shim.Error(fmt.Sprintf("failed to InvokeChaincode ccm %s: %s", string(ccm), resp.Message))
	}
	if err := stub.SetEvent(FromCCM, resp.Payload); err != nil {
		return shim.Error(fmt.Sprintf("failed to set event: %v", err))
	}

	logger.Infof("successful to call ccm for cross-chain: (to_chainID: %d, to_contract: %x, to_asset: %x, to_addr: %x, amount: %s)",
		chainId, toProxy, toAsset, toAddr, amt.String())

	return shim.Success(nil)
}

func (lp *LockProxy) unlock(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 1 {
		return shim.Error("args number should be 1")
	}
	ccname, err := utils.GetCallingChainCodeName(stub)
	if err != nil {
		return shim.Error(err.Error())
	}
	ccmName, _ := stub.GetState(ProxyCCM)
	if len(ccmName) == 0 {
		return shim.Error("No cross chain manager set")
	}
	if ccname != string(ccmName) {
		return shim.Error(fmt.Sprintf("wrong calling chaincode: (actual: %s, expected: %s)",
			ccname, string(ccmName)))
	}

	raw, err := hex.DecodeString(string(args[0]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode hex args: %v", err))
	}
	txArgs := &TxArgs{}
	if err := txArgs.Deserialization(pcommon.NewZeroCopySource(raw)); err != nil {
		return shim.Error(fmt.Sprintf("failed to deserialize tx args: %v", err))
	}
	lpAddr, err := stub.GetState(LockProxyAddr)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to get LockProxyAddr: %v", err))
	}

	transferArgs := make([][]byte, 4)
	transferArgs[0] = []byte(ProxyTransfer)
	transferArgs[1] = lpAddr
	transferArgs[2] = txArgs.ToAddress
	transferArgs[3] = txArgs.Amount.Bytes()
	resp := stub.InvokeChaincode(string(txArgs.ToAssetHash), transferArgs, "")
	if resp.Status != shim.OK {
		return shim.Error(fmt.Sprintf("failed to transfer %s from DApp address %x to address %x: %s",
			txArgs.Amount.String(), lpAddr, txArgs.ToAddress, resp.GetMessage()))
	}

	logger.Infof("unlock success: (to_addr: %x, amount: %s)", txArgs.ToAddress, txArgs.Amount.String())

	return shim.Success(nil)
}

func checkOwner(stub shim.ChaincodeStubInterface) ([]byte, error) {
	creator, err := utils.GetMsgSenderAddress(stub)
	if err != nil {
		return nil, err
	}
	owner, err := stub.GetState(ProxyOwner)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(creator.Bytes(), owner) {
		return nil, fmt.Errorf("is not owner")
	}
	return owner, nil
}

func getProxyBindKey(chainId uint64) string {
	return fmt.Sprintf(ProxyBindKey, chainId)
}

func getAssetBindKey(chainId uint64, fromAsset string) string {
	return fmt.Sprintf(AssetBindKey, chainId, fromAsset)
}
