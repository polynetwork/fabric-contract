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
	"fmt"
	"github.com/polynetwork/poly/consensus/vbft/config"
	"github.com/polynetwork/poly/core/signature"
	"github.com/polynetwork/poly/core/types"
	"github.com/polynetwork/poly/native/service/header_sync/ont"
)

func VerifyPolyHeader(hdr *types.Header, peers *ont.ConsensusPeers) error {
	if len(hdr.Bookkeepers)*3 < len(peers.PeerMap)*2 {
		return fmt.Errorf("header Bookkeepers num %d must more than 2/3 consensus node num %d",
			len(hdr.Bookkeepers), len(peers.PeerMap))
	}
	for i, bookkeeper := range hdr.Bookkeepers {
		pubkey := vconfig.PubkeyID(bookkeeper)
		_, present := peers.PeerMap[pubkey]
		if !present {
			return fmt.Errorf("No.%d pubkey is invalid: %s", i, pubkey)
		}
	}
	hash := hdr.Hash()
	if err := signature.VerifyMultiSignature(hash[:], hdr.Bookkeepers, len(hdr.Bookkeepers), hdr.SigData); err != nil {
		return fmt.Errorf("verify sig failed: %v", err)
	}

	return nil
}

type GenesisInitEvent struct {
	Height    uint32 `json:"height"`
	RawHeader []byte `json:"raw_header"`
}

type BookKeepersChangedEvent struct {
	RawPeers []byte `json:"raw_peers"`
}
