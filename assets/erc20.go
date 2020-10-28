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
package assets

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/polynetwork/fabric-contract/utils"
	common2 "github.com/polynetwork/poly/common"
	"io"
	"math/big"
	"strconv"
)

const (
	TokenId      = "ERC20TokenImpl"
	TokenOwner   = TokenId + "-Owner"
	TokenBalance = TokenId + "-%s-Balance"
	TokenFreeze  = TokenId + "-%s-Freeze"
	TokenApprove = TokenId + "-%s-Approve-%s"
	TokenName = TokenId + "-Name"
	TokenSymbol = TokenId + "-Symbol"
	TokenDecimal = TokenId + "-Deciaml"
	TokenTotalSupply = TokenId + "-TotalSupply"

	EventTranfer = TokenId + "transfer"
	EventApproval = TokenId + "approve"
	EventTransferOwnership = TokenId + "transferOwnerShip"

	ProxyCCM = "proxy_ccm"
	ProxyBindKey = "proxy-%d"
	AssetBindKey = "asset-%d"
	LockProxyAddr = "lockproxy_addr"
	ProxyChainCodeName = "chaincode_name"
	IsLockProxy = "is_lp"
)

type TxArgs struct {
	ToAssetHash []byte
	ToAddress []byte
	Amount uint64
}

func (args *TxArgs) Serialization(sink *common2.ZeroCopySink) {
	sink.WriteVarBytes(args.ToAssetHash)
	sink.WriteVarBytes(args.ToAddress)
	sink.WriteVarUint(args.Amount)
}

func (args *TxArgs) Deserialization(source *common2.ZeroCopySource) error {
	assetHash, eof := source.NextVarBytes()
	if eof {
		return fmt.Errorf("Args.Deserialization NextVarBytes AssetHash error:%s", io.ErrUnexpectedEOF)
	}

	toAddress, eof := source.NextVarBytes()
	if eof {
		return fmt.Errorf("Args.Deserialization NextVarBytes ToAddress error:%s", io.ErrUnexpectedEOF)
	}

	value, eof := source.NextVarUint()
	if eof {
		return fmt.Errorf("Args.Deserialization NextVarUint Value error:%s", io.ErrUnexpectedEOF)
	}
	args.ToAssetHash = assetHash
	args.ToAddress = toAddress
	args.Amount = value
	return nil
}

type ERC20 interface {
	// return with the name in bytes
	name(stub shim.ChaincodeStubInterface) pb.Response
	symbol(stub shim.ChaincodeStubInterface) pb.Response
	decimal(stub shim.ChaincodeStubInterface) pb.Response
	totalSupply(stub shim.ChaincodeStubInterface) pb.Response
	balanceOf(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response
	transfer(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response
	approve(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response
	transferFrom(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response
	allowance(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response
}

type ERC20TokenImpl struct {}

// args: name, symbol, decimal, totalsupply, chaincodeName, isLockProxy
func (token *ERC20TokenImpl) Init(stub shim.ChaincodeStubInterface) pb.Response {
	args := stub.GetStringArgs()
	if len(args) != 5 && len(args) != 6 {
		return shim.Error("wrong args number and should be five or six")
	}
	if args[0] == "" {
		return shim.Error(fmt.Sprintf("token name can't be empty"))
	}
	if args[1] == "" {
		return shim.Error(fmt.Sprintf("token symbol can't be empty"))
	}
	if len(args[4]) == 0 {
		return shim.Error(fmt.Sprintf("token chaincode name can't be empty"))
	}

	decimal, ok := big.NewInt(0).SetString(args[2], 10)
	if !ok {
		return shim.Error(fmt.Sprintf("failed to decode decimal from string: %s", args[2]))
	}
	if decimal.Sign() != 1 {
		return shim.Error(fmt.Sprintf("token decimal must be must be positive"))
	}

	totalSupply, ok := big.NewInt(0).SetString(args[3], 10)
	if !ok {
		return shim.Error(fmt.Sprintf("failed to decode totalSupply: %s", args[3]))
	}
	if totalSupply.Sign() != 1 {
		return shim.Error(fmt.Sprintf("token totalsupply must be positive"))
	}

	if err := stub.PutState(TokenName, []byte(args[0])); err != nil {
		return shim.Error(fmt.Sprintf("failed To put token name: %v", err))
	}
	if err := stub.PutState(TokenSymbol, []byte(args[1])); err != nil {
		return shim.Error(fmt.Sprintf("failed To put token symbol: %v", err))
	}
	if err := stub.PutState(TokenDecimal, decimal.Bytes()); err != nil {
		return shim.Error(fmt.Sprintf("failed To put token decimal: %v", err))
	}
	if err := stub.PutState(TokenTotalSupply, totalSupply.Bytes()); err != nil {
		return shim.Error(fmt.Sprintf("failed To put token totalsupply: %v", err))
	}

	owner, err := utils.GetMsgSenderAddress(stub)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed To get tx sender: %v", err))
	}
	if err = stub.PutState(TokenOwner, owner.Bytes()); err != nil {
		return shim.Error(fmt.Sprintf("failed To put token owner: %v", err))
	}
	lpAddr := common.BytesToAddress(crypto.Keccak256(append([]byte(LockProxyAddr), owner.Bytes()...))[12:]).Bytes()
	if err := stub.PutState(LockProxyAddr, lpAddr); err != nil {
		return shim.Error(fmt.Sprintf("failed to put lockproxy addr: %v", err))
	}
	if err := stub.PutState(ProxyChainCodeName, []byte(args[4])); err != nil {
		return shim.Error(fmt.Sprintf("failed to save chaincode name: %v", err))
	}

	var holder []byte
	// if we get six args and it's true, it means this ERC20 start with LockProxy func
	if len(args) == 6 && args[5] == "true" {
		if err := stub.PutState(IsLockProxy, []byte(args[5])); err != nil {
			return shim.Error(fmt.Sprintf("failed to save is_lockproxy: %v", err))
		}
		holder = lpAddr
	} else {
		holder = owner.Bytes()
	}

	if err = stub.PutState(balanceKey(holder), totalSupply.Bytes()); err != nil {
		return shim.Error("failed To put all token To holder")
	}
	return shim.Success(nil)
}

func (token *ERC20TokenImpl) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	fn, _ := stub.GetFunctionAndParameters()
	args := stub.GetArgs()
	if len(args) == 0 {
		return shim.Error("no args")
	}
	args = args[1:]

	switch fn {
	case "name":
		return token.name(stub)
	case "symbol":
		return token.symbol(stub)
	case "decimal":
		return token.decimal(stub)
	case "totalSupply":
		return token.totalSupply(stub)
	case "getMyAddr":
		return token.getMyAddr(stub)
	case "getOwner":
		return token.getOwner(stub)
	case "getLockProxyAddr":
		return token.getLockProxyAddr(stub)
	case "isCrossChainOn":
		return token.isCrossChainOn(stub)
	case "balanceOf":
		return token.balanceOf(stub, args)
	case "mint":
		return token.mint(stub, args)
	case "transfer":
		return token.transfer(stub, args)
	case "approve":
		return token.approve(stub, args)
	case "transferFrom":
		return token.transferFrom(stub, args)
	case "allowance":
		return token.allowance(stub, args)
	case "transferOwnership":
		return token.transferOwnership(stub, args)
	case "increaseAllowance":
		return token.increaseAllowance(stub, args)
	case "decreaseAllowance":
		return token.decreaseAllowance(stub, args)
	case "burn":
		return token.burn(stub, args)
	case "setManager":
		return token.setManager(stub, args)
	case "bindProxyHash":
		return token.bindProxyHash(stub, args)
	case "getProxyHash":
		return token.getProxyHash(stub, args)
	case "bindAssetHash":
		return token.bindAssetHash(stub, args)
	case "getAssetHash":
		return token.getAssetHash(stub, args)
	case "lock":
		return token.lock(stub, args)
	case "unlock":
		return token.unlock(stub, args)
	}

	return shim.Error(fmt.Sprintf("no function name %s found", fn))
}

func (token *ERC20TokenImpl) name(stub shim.ChaincodeStubInterface) pb.Response {
	rawName, err := stub.GetState(TokenName)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(rawName)
}

func (token *ERC20TokenImpl) symbol(stub shim.ChaincodeStubInterface) pb.Response {
	raw, err := stub.GetState(TokenSymbol)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(raw)
}

func (token *ERC20TokenImpl) decimal(stub shim.ChaincodeStubInterface) pb.Response {
	raw, err := stub.GetState(TokenDecimal)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(raw)
}

func (token *ERC20TokenImpl) totalSupply(stub shim.ChaincodeStubInterface) pb.Response {
	raw, err := stub.GetState(TokenTotalSupply)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(raw)
}

func (token *ERC20TokenImpl) balanceOf(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 1 {
		return shim.Error("args number should be 1")
	}
	acc, err := hex.DecodeString(string(args[0]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode hex holder: %v", err))
	}
	balance, _ := stub.GetState(balanceKey(acc))
	if len(balance) == 0 {
		return shim.Success(big.NewInt(0).Bytes())
	}
	return shim.Success(balance)
}

func (token *ERC20TokenImpl) mint(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 2 {
		return shim.Error("number of args should be 2")
	}
	if _, err := checkOwner(stub); err != nil {
		return shim.Error(fmt.Sprintf("failed To check owner: %v", err))
	}

	amt, ok := big.NewInt(0).SetString(string(args[1]), 10)
	if !ok {
		return shim.Error(fmt.Sprintf("failed to decode amount: %s", args[1]))
	}
	if amt.Sign() != 1 {
		return shim.Error("amount should be positive")
	}

	rawAcc, err := hex.DecodeString(string(args[0]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode hex account: %v", err))
	}
	key := balanceKey(rawAcc)
	rawBal, err := stub.GetState(key)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed To get balance: %v", err))
	}
	bal := big.NewInt(0).SetBytes(rawBal)
	bal = bal.Add(bal, amt)
	
	rawSupply, err := stub.GetState(TokenTotalSupply)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed To get totalsupply: %v", err))
	}
	ts := big.NewInt(0).SetBytes(rawSupply)
	ts = ts.Add(ts, amt)
	
	if err := stub.PutState(key, bal.Bytes()); err != nil {
		return shim.Error(fmt.Sprintf("failed To update balance: %v", err))
	}
	if err := stub.PutState(TokenTotalSupply, ts.Bytes()); err != nil {
		return shim.Error(fmt.Sprintf("failed To update totalsupply: %v", err))
	}
	return shim.Success(nil)
}

func (token *ERC20TokenImpl) transfer(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 2 {
		return shim.Error("number of transferLogic args should be 2")
	}
	from, err := utils.GetMsgSenderAddress(stub)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed To get tx sender: %v", err))
	}
	to, err := hex.DecodeString(string(args[0]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode hex account: %v", err))
	}
	amt, ok := big.NewInt(0).SetString(string(args[1]), 10)
	if !ok {
		return shim.Error(fmt.Sprintf("failed to decode amount: %s", args[1]))
	}
	return token.transferLogic(stub, from.Bytes(), to, amt)
}

func (token *ERC20TokenImpl) transferLogic(stub shim.ChaincodeStubInterface, from, to []byte, amt *big.Int) pb.Response {
	if amt.Sign() != 1 {
		return shim.Error("amount should be positive")
	}

	fromKey := balanceKey(from)
	rawFromBal, err := stub.GetState(fromKey)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed To get From balance: %v", err))
	}
	fromBal := big.NewInt(0).SetBytes(rawFromBal)
	fromBal = fromBal.Sub(fromBal, amt)
	if fromBal.Sign() == -1 {
		return shim.Error(fmt.Sprintf("From balance %s is less than the amount %s", fromBal.String(), amt.String()))
	}

	toKey := balanceKey(to)
	rawToBal, err := stub.GetState(toKey)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed To get receive account balance: %v", err))
	}
	toBal := big.NewInt(0).SetBytes(rawToBal)
	toBal = toBal.Add(toBal, amt)

	if fromBal.Sign() == 0 {
		if err := stub.DelState(fromKey); err != nil {
			return shim.Error(fmt.Sprintf("failed To delete balance for From account: %v", err))
		}
	} else {
		if err := stub.PutState(fromKey, fromBal.Bytes()); err != nil {
			return shim.Error(fmt.Sprintf("failed To put balance for From account: %v", err))
		}
	}
	if err := stub.PutState(toKey, toBal.Bytes()); err != nil {
		return shim.Error(fmt.Sprintf("failed To put balance for receiver account: %v", err))
	}

	rawEvent, err := json.Marshal(&TransferEvent{
		From: from,
		To: to,
		Amount: amt.Bytes(),
	})
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to json marshal: %v", err))
	}
	if err := stub.SetEvent(EventTranfer, rawEvent); err != nil {
		return shim.Error(fmt.Sprintf("failed to set event: %v", err))
	}

	return shim.Success(nil)
}

func (token *ERC20TokenImpl) transferOwnership(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
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
	if err := stub.PutState(TokenOwner, rawAcc); err != nil {
		return shim.Error(err.Error())
	}
	rawEvent, err := json.Marshal(&TransferOwnershipEvent{
		NewOwner: rawAcc,
		OldOwner: old,
	})
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to json marshal: %v", err))
	}
	if err := stub.SetEvent(EventTransferOwnership, rawEvent); err != nil {
		return shim.Error(fmt.Sprintf("failed to set event: %v", err))
	}
	return shim.Success(nil)
}

func (token *ERC20TokenImpl) approve(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 2 {
		return shim.Error("number of args should be 2")
	}
	from, err := utils.GetMsgSenderAddress(stub)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed To get tx sender: %v", err))
	}
	spender, err := hex.DecodeString(string(args[0]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode hex spender: %v", err))
	}
	amt, ok := big.NewInt(0).SetString(string(args[1]), 10)
	if !ok {
		return shim.Error(fmt.Sprintf("failed to decode amount: %s", args[1]))
	}
	if amt.Sign() != 1 {
		return shim.Error(fmt.Sprintf("amount should be positive: %s", amt.String()))
	}
	rawAmt := amt.Bytes()
	if err = stub.PutState(approveKey(from.Bytes(), spender), rawAmt); err != nil {
		return shim.Error(err.Error())
	}

	rawEvent, err := json.Marshal(&ApprovalEvent{
		Amount: rawAmt,
		From: from.Bytes(),
		Spender: spender,
	})
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to json marshal: %v", err))
	}
	if err := stub.SetEvent(EventApproval, rawEvent); err != nil {
		return shim.Error(fmt.Sprintf("failed to set event: %v", err))
	}

	return shim.Success(nil)
}

func (token *ERC20TokenImpl) transferFrom(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 3 {
		return shim.Error("number of args should be 3")
	}
	spender, err := utils.GetMsgSenderAddress(stub)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed To get tx sender: %v", err))
	}
	amt, ok := big.NewInt(0).SetString(string(args[2]), 10)
	if !ok {
		return shim.Error(fmt.Sprintf("failed to decode amount: %s", args[2]))
	}

	from, err := hex.DecodeString(string(args[0]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode hex from: %v", err))
	}
	key := approveKey(from, spender.Bytes())
	raw, err := stub.GetState(key)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed To get approve value for %s: %v", key, err))
	}
	val := big.NewInt(0).SetBytes(raw)
	leftVal := val.Sub(val, amt)
	if leftVal.Sign() == -1 {
		return shim.Error(fmt.Sprintf("approved value %s is not enough To pay %s", val.String(), amt.String()))
	} else if leftVal.Sign() == 0 {
		if err := stub.DelState(key); err != nil {
			return shim.Error(fmt.Sprintf("delete %s failed: %v", key, err))
		}
	} else {
		if err = stub.PutState(key, leftVal.Bytes()); err != nil {
			return shim.Error(fmt.Sprintf("failed To put %s: %v", key, err))
		}
	}

	to, err := hex.DecodeString(string(args[1]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode hex to_addr: %v", err))
	}

	return token.transferLogic(stub, from, to, amt)
}

func (token *ERC20TokenImpl) allowance(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 2 {
		return shim.Error("number of args should be 2")
	}
	from, err := hex.DecodeString(string(args[0]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode hex from: %v", err))
	}
	spender, err := hex.DecodeString(string(args[1]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode hex spender: %v", err))
	}
	val, err := stub.GetState(approveKey(from, spender))
	if err != nil {
		return shim.Error(err.Error())
	}
	return shim.Success(val)
}

func (token *ERC20TokenImpl) increaseAllowance(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 2 {
		return shim.Error("number of args should be 2")
	}
	spender, err := hex.DecodeString(string(args[0]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode hex spender: %v", err))
	}
	amt, ok := big.NewInt(0).SetString(string(args[1]), 10)
	if !ok {
		return shim.Error(fmt.Sprintf("failed to decode amount: %s", args[1]))
	}
	return token.changeAllowance(stub, spender, amt)
}

func (token *ERC20TokenImpl) decreaseAllowance(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 2 {
		return shim.Error("number of args should be 2")
	}
	spender, err := hex.DecodeString(string(args[0]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode hex spender: %v", err))
	}
	amt, ok := big.NewInt(0).SetString(string(args[1]), 10)
	if !ok {
		return shim.Error(fmt.Sprintf("failed to decode amount: %s", args[1]))
	}
	amt = amt.Neg(amt)
	return token.changeAllowance(stub, spender, amt)
}

func (token *ERC20TokenImpl) changeAllowance(stub shim.ChaincodeStubInterface, spender []byte, amt *big.Int) pb.Response {
	if amt.Sign() == 0 {
		return shim.Error("amount can't be zero")
	}

	from, err := utils.GetMsgSenderAddress(stub)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed To get tx sender: %v", err))
	}
	key := approveKey(from.Bytes(), spender)
	raw, err := stub.GetState(key)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed To get approved value for %s: %v", key, err))
	}
	val := big.NewInt(0).SetBytes(raw)
	leftVal := val.Add(val, amt)

	if leftVal.Sign() == -1 {
		return shim.Error(fmt.Sprintf("approved value %s is not enough To decrease %s", val.String(), amt.String()))
	} else if leftVal.Sign() == 0 {
		if err := stub.DelState(key); err != nil {
			return shim.Error(fmt.Sprintf("delete %s failed: %v", key, err))
		}
	} else {
		if err = stub.PutState(key, leftVal.Bytes()); err != nil {
			return shim.Error(fmt.Sprintf("failed To put %s: %v", key, err))
		}
	}

	return shim.Success(nil)
}

func (token *ERC20TokenImpl) burn(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 1 {
		return shim.Error("number of args should be 1")
	}
	from, err := utils.GetMsgSenderAddress(stub)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed To get tx sender: %v", err))
	}
	rawSupply, err := stub.GetState(TokenTotalSupply)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed To get totalsupply: %v", err))
	}
	ts := big.NewInt(0).SetBytes(rawSupply)

	amt, ok := big.NewInt(0).SetString(string(args[1]), 10)
	if !ok {
		return shim.Error(fmt.Sprintf("failed to decode amount: %s", args[1]))
	}
	if amt.Sign() == 0 {
		return shim.Error("amount can't be zero")
	}
	ts = ts.Sub(ts, amt)

	if err := stub.PutState(TokenTotalSupply, ts.Bytes()); err != nil {
		return shim.Error(fmt.Sprintf("failed To update totalsupply: %v", err))
	}
	return token.transferLogic(stub, from.Bytes(), common.Address{}.Bytes(), amt)
}

func (token *ERC20TokenImpl) setManager(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 1 {
		return shim.Error("args number should be 1")
	}
	if !isCCOn(stub) {
		return shim.Error("it's not for crosschain")
	}
	if _, err := checkOwner(stub); err != nil {
		return shim.Error(fmt.Sprintf("failed to check owner: %v", err))
	}
	if err := stub.PutState(ProxyCCM, args[0]); err != nil {
		return shim.Error(fmt.Sprintf("failed to put cross chain manager name: %v", err))
	}
	return shim.Success(nil)
}

func (token *ERC20TokenImpl) bindProxyHash(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 2 {
		return shim.Error("args number should be 2")
	}
	if !isCCOn(stub) {
		return shim.Error("it's not for crosschain")
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

func (token *ERC20TokenImpl) getProxyHash(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
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

func (token *ERC20TokenImpl) bindAssetHash(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 2 {
		return shim.Error("args number should be 2")
	}
	if !isCCOn(stub) {
		return shim.Error("it's not for crosschain")
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
		return shim.Error(fmt.Sprintf("failed to decode hex target asset: %v", err))
	}
	if err := stub.PutState(getAssetBindKey(chainId), target); err != nil {
		return shim.Error(fmt.Sprintf("failed to put asset: %v", err))
	}

	return shim.Success(nil)
}

func (token *ERC20TokenImpl) getAssetHash(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 1 {
		return shim.Error("args number should be 1")
	}
	chainId, err := strconv.ParseUint(string(args[0]), 10, 64)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to parse chainId: %v", err))
	}
	val, err := stub.GetState(getAssetBindKey(chainId))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to put asset: %v", err))
	}

	return shim.Success(val)
}

func (token *ERC20TokenImpl) lock(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	if len(args) != 3 {
		return shim.Error("args number should be 4")
	}
	if !isCCOn(stub) {
		return shim.Error("it's not for crosschain")
	}

	lpAddr, err := stub.GetState(LockProxyAddr)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to get LockProxyAddr: %v", err))
	}

	from, err := utils.GetMsgSenderAddress(stub)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to get tx sender: %v", err))
	}
	amt, ok := big.NewInt(0).SetString(string(args[2]), 10)
	if !ok {
		return shim.Error(fmt.Sprintf("failed to decode amount: %s", args[2]))
	}
	if amt.Sign() != 1 {
		return shim.Error(fmt.Sprintf("amount should be positive"))
	}

	resp := token.transferLogic(stub, from.Bytes(), lpAddr, amt)
	if resp.Status != shim.OK {
		return shim.Error(fmt.Sprintf("failed to lock asset: %v", err))
	}

	chainId, err := strconv.ParseUint(string(args[0]), 10, 64)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to parse chainId: %v", err))
	}
	toAsset, err := stub.GetState(getAssetBindKey(chainId))
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

	toAddr, err := hex.DecodeString(string(args[1]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode hex toAddr: %v", err))
	}
	txArgs := &TxArgs{
		ToAssetHash: toAsset,
		ToAddress: toAddr,
		Amount: amt.Uint64(),
	}
	sink := common2.NewZeroCopySink(nil)
	txArgs.Serialization(sink)

	ccname, err := stub.GetState(ProxyChainCodeName)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to get chaincode name: %v", err))
	}

	invokeArgs := make([][]byte, 6)
	invokeArgs[0] = []byte("crossChain")
	invokeArgs[1] = args[0]
	invokeArgs[2] = []byte(hex.EncodeToString(toProxy))
	invokeArgs[3] = []byte("unlock")
	invokeArgs[4] = []byte(hex.EncodeToString(sink.Bytes()))
	invokeArgs[5] = ccname

	return stub.InvokeChaincode(string(ccm), invokeArgs, stub.GetChannelID())
}

func (token *ERC20TokenImpl) unlock(stub shim.ChaincodeStubInterface, args [][]byte) pb.Response {
	// TODO: limit the caller must be the ccm
	if len(args) != 1 {
		return shim.Error("args number should be 1")
	}
	if !isCCOn(stub) {
		return shim.Error("it's not for crosschain")
	}
	raw, err := hex.DecodeString(string(args[0]))
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to decode hex args: %v", err))
	}
	txArgs := &TxArgs{}
	if err := txArgs.Deserialization(common2.NewZeroCopySource(raw)); err != nil {
		return shim.Error(fmt.Sprintf("failed to deserialize tx args: %v", err))
	}
	lpAddr, err := stub.GetState(LockProxyAddr)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to get LockProxyAddr: %v", err))
	}

	return token.transferLogic(stub, lpAddr, txArgs.ToAddress, big.NewInt(0).SetUint64(txArgs.Amount))
}

func (token *ERC20TokenImpl) getOwner(stub shim.ChaincodeStubInterface) pb.Response {
	owner, err := stub.GetState(TokenOwner)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(owner)
}

func (token *ERC20TokenImpl) getMyAddr(stub shim.ChaincodeStubInterface) pb.Response {
	creator, err := utils.GetMsgSenderAddress(stub)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to get sender: %v", err))
	}
	return shim.Success(creator.Bytes())
}

func (token *ERC20TokenImpl) getLockProxyAddr(stub shim.ChaincodeStubInterface) pb.Response {
	lpAddr, err := stub.GetState(LockProxyAddr)
	if err != nil {
		return shim.Error(err.Error())
	}
	return shim.Success(lpAddr)
}

func (token *ERC20TokenImpl) isCrossChainOn(stub shim.ChaincodeStubInterface) pb.Response {
	val, _ := stub.GetState(IsLockProxy)
	if len(val) == 0 {
		return shim.Success([]byte("false"))
	}
	return shim.Success(val)
}

func isCCOn(stub shim.ChaincodeStubInterface) bool {
	val, _ := stub.GetState(IsLockProxy)
	if len(val) == 0 {
		return false
	}
	return true
}

func checkOwner(stub shim.ChaincodeStubInterface) ([]byte, error) {
	creator, err := utils.GetMsgSenderAddress(stub)
	if err != nil {
		return nil, err
	}
	owner, err := stub.GetState(TokenOwner)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(creator.Bytes(), owner) {
		return nil, fmt.Errorf("is not owner")
	}
	return owner, nil
}

func balanceKey(acc []byte) string {
	return fmt.Sprintf(TokenBalance, hex.EncodeToString(acc))
}

func approveKey(from, spender []byte) string {
	return fmt.Sprintf(TokenApprove, hex.EncodeToString(from), hex.EncodeToString(spender))
}

func getProxyBindKey(chainId uint64) string {
	return fmt.Sprintf(ProxyBindKey, chainId)
}

func getAssetBindKey(chainId uint64) string {
	return fmt.Sprintf(AssetBindKey, chainId)
}

