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
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/hyperledger/fabric/core/chaincode/shim"
)

func GetMsgSenderAddress(stub shim.ChaincodeStubInterface) (common.Address, error) {
	creatorByte, err := stub.GetCreator()
	if err != nil {
		return common.Address{}, err
	}
	certStart := bytes.Index(creatorByte, []byte("-----BEGIN"))
	if certStart == -1 {
		return common.Address{}, fmt.Errorf("no CA found")
	}
	certText := creatorByte[certStart:]
	bl, _ := pem.Decode(certText)
	if bl == nil {
		return common.Address{}, fmt.Errorf("failed to decode pem")
	}

	cert, err := x509.ParseCertificate(bl.Bytes)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to parse CA: %v", err)
	}
	hash := sha256.New()
	hash.Write(cert.RawSubjectPublicKeyInfo)
	addr := common.BytesToAddress(hash.Sum(nil)[12:])
	return addr, nil
	//switch pub := cert.PublicKey.(type) {
	//case *rsa.PublicKey:
	//	pub.
	//case *dsa.PublicKey:
	//	fmt.Println("pub is of type DSA:", pub)
	//case *ecdsa.PublicKey:
	//	fmt.Println("pub is of type ECDSA:", pub)
	//case ed25519.PublicKey:
	//	fmt.Println("pub is of type Ed25519:", pub)
	//default:
	//	panic("unknown type of public key")
	//}
}
