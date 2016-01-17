// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
)

// The databaseUpdater performs database updates in batches based on
// Progresser updates.
type databaseUpdater struct {
	folder           string
	model            *Model
	updates          chan protocol.FileInfo
	batch            []protocol.FileInfo
	lastReceivedFile protocol.FileInfo
	done             chan struct{} // closed when Close() has been called and all pending operations have been committed
}

func newDatabaseUpdater(folder string, model *Model) *databaseUpdater {
	p := &databaseUpdater{
		folder:  folder,
		model:   model,
		updates: make(chan protocol.FileInfo),
		done:    make(chan struct{}),
	}
	go p.runner()
	return p
}

func (p *databaseUpdater) Started(file protocol.FileInfo) {
	// Don't care
}

func (p *databaseUpdater) Progress(file protocol.FileInfo, copied, requested, downloaded int) {
	// Don't care
}

func (p *databaseUpdater) Completed(file protocol.FileInfo, err error) {
	if err == nil {
		file.LocalVersion = 0
		p.updates <- file
	}
}

// Close stops the databaseUpdater from accepting further changes and
// awaits commit of all pending operations before returning.
func (p *databaseUpdater) Close() {
	close(p.updates)
	<-p.done
}

func (p *databaseUpdater) runner() {
	const (
		maxBatchSize = 1000
		maxBatchTime = 2 * time.Second
	)

	defer close(p.done)

	p.batch = make([]protocol.FileInfo, 0, maxBatchSize)
	nextCommit := time.NewTimer(maxBatchTime)
	defer nextCommit.Stop()

loop:
	for {
		select {
		case update, ok := <-p.updates:
			if !ok {
				break loop
			}

			if !update.IsDirectory() && !update.IsDeleted() && !update.IsInvalid() && !update.IsSymlink() {
				p.lastReceivedFile = update
			}

			p.batch = append(p.batch, update)
			if len(p.batch) == maxBatchSize {
				p.commit()
				nextCommit.Reset(maxBatchTime)
			}

		case <-nextCommit.C:
			if len(p.batch) > 0 {
				p.commit()
			}
			nextCommit.Reset(maxBatchTime)
		}
	}

	if len(p.batch) > 0 {
		p.commit()
	}
}

func (p *databaseUpdater) commit() {
	if shouldDebug() {
		l.Debugln("databaseUpdater.commit() committing batch of size", len(p.batch))
	}
	p.model.updateLocals(p.folder, p.batch)
	if p.lastReceivedFile.Name != "" {
		p.model.receivedFile(p.folder, p.lastReceivedFile)
		p.lastReceivedFile = protocol.FileInfo{}
	}

	for i := range p.batch {
		// Clear out the existing structures to the garbage collector can free
		// their block lists and so on.
		p.batch[i] = protocol.FileInfo{}
	}
	p.batch = p.batch[:0]
}

type localBlockPuller struct {
	model       *Model
	folders     []string
	folderRoots map[string]string
}

func (p *localBlockPuller) Request(file string, offset int64, hash []byte, buf []byte) error {
	fn := func(folder, file string, index int32) bool {
		fd, err := os.Open(filepath.Join(p.folderRoots[folder], file))
		if err != nil {
			return false
		}

		_, err = fd.ReadAt(buf, protocol.BlockSize*int64(index))
		fd.Close()
		if err != nil {
			return false
		}

		actualHash, err := scanner.VerifyBuffer(buf, protocol.BlockInfo{
			Size: int32(len(buf)),
			Hash: hash,
		})
		if err != nil {
			if hash != nil {
				l.Debugf("Finder block mismatch in %s:%s:%d expected %q got %q", folder, file, index, hash, actualHash)
				err = p.model.finder.Fix(folder, file, index, hash, actualHash)
				if err != nil {
					l.Warnln("finder fix:", err)
				}
			} else {
				l.Debugln("Finder failed to verify buffer", err)
			}
			return false
		}
		return true
	}

	if p.model.finder.Iterate(p.folders, hash, fn) {
		return nil
	}

	return errors.New("no such block")
}

type networkBlockPuller struct {
	model  *Model
	folder string
}

func (p *networkBlockPuller) Request(file string, offset int64, hash []byte, buf []byte) error {
	potentialDevices := p.model.Availability(p.folder, file)
	for {
		// Select the least busy device to pull the block from. If we found no
		// feasible device at all, fail the block (and in the long run, the
		// file).
		selected := activity.leastBusy(potentialDevices)
		if selected == (protocol.DeviceID{}) {
			l.Debugln("request:", p.folder, file, offset, len(buf), errNoDevice)
			return errNoDevice
		}

		potentialDevices = removeDevice(potentialDevices, selected)

		// Fetch the block, while marking the selected device as in use so that
		// leastBusy can select another device when someone else asks.
		activity.using(selected)
		tmpBbuf, err := p.model.requestGlobal(selected, p.folder, file, offset, len(buf), hash, 0, nil)
		activity.done(selected)
		if err != nil {
			l.Debugln("request:", p.folder, file, offset, len(buf), "returned error:", err)
			return err
		}

		// Verify that the received block matches the desired hash, if not
		// try pulling it from another device.
		_, err = scanner.VerifyBuffer(tmpBbuf, protocol.BlockInfo{
			Size: int32(len(buf)),
			Hash: hash,
		})
		if err != nil {
			l.Debugln("request:", p.folder, file, offset, len(buf), err)
			continue
		}

		l.Debugln("completed request:", p.folder, file, offset, len(buf))
		copy(buf, tmpBbuf)
		break
	}

	return nil
}
