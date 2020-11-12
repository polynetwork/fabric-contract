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
	"errors"
	"fmt"
	"github.com/polynetwork/poly/common"
	pcom "github.com/polynetwork/poly/native/service/cross_chain_manager/common"
)

type TransferEvent struct {
	From   []byte `json:"from"`
	To     []byte `json:"to"`
	Amount []byte `json:"amount"`
}

type ApprovalEvent struct {
	From    []byte `json:"from"`
	Spender []byte `json:"spender"`
	Amount  []byte `json:"amount"`
}

type TransferOwnershipEvent struct {
	OldOwner []byte `json:"old_owner"`
	NewOwner []byte `json:"new_owner"`
}

func GetWhatCCMCalling(rawProof []byte) (string, error) {
	value, _, _, err := ParseAuditpath(rawProof)
	if err != nil {
		return "", err
	}
	merkleValue := new(pcom.ToMerkleValue)
	if err := merkleValue.Deserialization(common.NewZeroCopySource(value)); err != nil {
		return "", fmt.Errorf("deserialize merkleValue error: %v", err)
	}
	return string(merkleValue.MakeTxParam.ToContractAddress), nil
}

func ParseAuditpath(path []byte) ([]byte, []byte, [][32]byte, error) {
	source := common.NewZeroCopySource(path)
	value, eof := source.NextVarBytes()
	if eof {
		return nil, nil, nil, errors.New("NextVarBytes eof")
	}
	size := int((source.Size() - source.Pos()) / common.UINT256_SIZE)
	pos := make([]byte, 0)
	hashs := make([][32]byte, 0)
	for i := 0; i < size; i++ {
		f, eof := source.NextByte()
		if eof {
			return nil, nil, nil, errors.New("NextByte eof")
		}
		pos = append(pos, f)

		v, eof := source.NextHash()
		if eof {
			return nil, nil, nil, errors.New("NextHash eof")
		}
		var onehash [32]byte
		copy(onehash[:], (v.ToArray())[0:32])
		hashs = append(hashs, onehash)
	}

	return value, pos, hashs, nil
}