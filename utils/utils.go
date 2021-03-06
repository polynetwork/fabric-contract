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
package utils

import (
	"crypto/sha256"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/hyperledger/fabric/core/chaincode/lib/cid"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/protos/utils"
	"math/big"
)

func GetMsgSenderAddress(stub shim.ChaincodeStubInterface) (common.Address, error) {
	cert, err := cid.GetX509Certificate(stub)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to parse CA: %v", err)
	}
	return GetAddrFromRaw(cert.RawSubjectPublicKeyInfo), nil
}

func GetAddrFromRaw(raw []byte) common.Address {
	hash := sha256.New()
	hash.Write(raw)
	addr := common.BytesToAddress(hash.Sum(nil)[12:])
	return addr
}

func BigIntFromNeoBytes(ba []byte) *big.Int {
	res := big.NewInt(0)
	l := len(ba)
	if l == 0 {
		return res
	}

	bytes := make([]byte, 0, l)
	bytes = append(bytes, ba...)
	bytesReverse(bytes)

	if bytes[0]>>7 == 1 {
		for i, b := range bytes {
			bytes[i] = ^b
		}

		temp := big.NewInt(0)
		temp.SetBytes(bytes)
		temp.Add(temp, big.NewInt(1))
		bytes = temp.Bytes()
		res.SetBytes(bytes)
		return res.Neg(res)
	}

	res.SetBytes(bytes)
	return res
}

func BigIntToNeoBytes(data *big.Int) []byte {
	bs := data.Bytes()
	if len(bs) == 0 {
		return []byte{}
	}
	// golang big.Int use big-endian
	bytesReverse(bs)
	// bs now is little-endian
	if data.Sign() < 0 {
		for i, b := range bs {
			bs[i] = ^b
		}
		for i := 0; i < len(bs); i++ {
			if bs[i] == 255 {
				bs[i] = 0
			} else {
				bs[i] += 1
				break
			}
		}
		if bs[len(bs)-1] < 128 {
			bs = append(bs, 255)
		}
	} else {
		if bs[len(bs)-1] >= 128 {
			bs = append(bs, 0)
		}
	}
	return bs
}

func bytesReverse(u []byte) []byte {
	for i, j := 0, len(u)-1; i < j; i, j = i+1, j-1 {
		u[i], u[j] = u[j], u[i]
	}
	return u
}

func GetCallingChainCodeName(stub shim.ChaincodeStubInterface) (string, error) {
	sp, err := stub.GetSignedProposal()
	if err != nil {
		return "", fmt.Errorf("failed to get signed proposal: %v", err)
	}
	p, err := utils.GetProposal(sp.ProposalBytes)
	if err != nil {
		return "", fmt.Errorf("failed to decode proposal: %v", err)
	}
	spec, err := utils.GetChaincodeInvocationSpec(p)
	if err != nil {
		return "", fmt.Errorf("failed to get invocation spec: %v", err)
	}
	return spec.ChaincodeSpec.ChaincodeId.Name, nil
}

func GetOriginalInputArgs(stub shim.ChaincodeStubInterface) ([][]byte, error) {
	sp, err := stub.GetSignedProposal()
	if err != nil {
		return nil, fmt.Errorf("failed to get signed proposal: %v", err)
	}
	p, err := utils.GetProposal(sp.ProposalBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to decode proposal: %v", err)
	}
	spec, err := utils.GetChaincodeInvocationSpec(p)
	if err != nil {
		return nil, fmt.Errorf("failed to get invocation spec: %v", err)
	}
	return spec.ChaincodeSpec.Input.Args, nil
}
