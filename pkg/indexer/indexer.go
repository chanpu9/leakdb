package indexer

/*
	---------------------------------------------------------------------
	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU General Public License for more details.

	You should have received a copy of the GNU General Public License
	along with this program.  If not, see <http://www.gnu.org/licenses/>.
	----------------------------------------------------------------------

	[48-bit digest][48-bit offset] = 96-bit (12 byte) entry

*/

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	kb = 1024
	mb = kb * 1024
	gb = mb * 1024

	digestSize = 6
	offsetSize = 6
)

// Worker - Worker thread
type Worker struct {
	ID         int
	Wg         *sync.WaitGroup
	TargetPath string
	OutputPath string
	Verbose    bool
	Labor      Labor
}

// Credential - JSON parsed line
type Credential struct {
	Email    string
	User     string
	Domain   string
	Password string
}

// Line - Raw data of a line in the file and offset
type Line struct {
	Raw    string
	Offset int64
}

// Labor - Each worker's part of a file
type Labor struct {
	Start int64
	Stop  int64
}

// Cred - Prase the raw line as a Credential
func (l *Line) Cred() Credential {
	var cred Credential
	json.Unmarshal([]byte(l.Raw), &cred)
	return cred
}

func (w *Worker) start(key string) {

	outputFile, err := os.Create(w.OutputPath)
	if err != nil {
		panic(err)
	}
	targetFile, err := os.Open(w.TargetPath)
	if err != nil {
		panic(err)
	}

	go func() {
		defer func() {
			outputFile.Close()
			targetFile.Close()
			w.Wg.Done()
		}()

		position := int64(w.Labor.Start)
		targetFile.Seek(position, 0)
		scanner := bufio.NewScanner(targetFile)

		for scanner.Scan() {
			rawLine := scanner.Text()
			line := &Line{
				Raw:    rawLine,
				Offset: position,
			}
			cred := line.Cred()
			value, _ := getKeyValue(cred, key)
			digest := sha256.Sum256([]byte(value))
			offsetBuf := make([]byte, 8)
			binary.LittleEndian.PutUint64(offsetBuf, uint64(line.Offset))
			outputFile.Write(digest[:digestSize])
			outputFile.Write(offsetBuf[:offsetSize])
			if w.Verbose {
				fmt.Printf("%d) [%d] '%s' -> (%x : %x)\n", w.ID, position, value, digest[:digestSize], offsetBuf[:offsetSize])
			}
			position += int64(len(rawLine) + 1)
			if w.Labor.Stop <= position {
				if w.Verbose {
					fmt.Printf("%d) Pos = %d, Stop = %d\n", w.ID, w.Labor.Stop, position)
				}
				break
			}
		}
	}()

}

func getKeyValue(cred Credential, key string) (string, error) {
	switch key {
	case "email":
		return cred.Email, nil
	case "domain":
		return cred.Domain, nil
	case "user":
		return cred.User, nil
	case "password":
		return cred.Password, nil
	}
	return "", fmt.Errorf("invalid index key '%s'", key)
}

func mergeIndexes(output string, indexDir string, noCleanup bool) error {
	outputFile, err := os.Create(output)
	if err != nil {
		return err
	}
	defer func() {
		outputFile.Close()
	}()

	indexFiles, err := ioutil.ReadDir(indexDir)
	if err != nil {
		return err
	}

	fmt.Printf("Merging indexes ... ")

	for _, indexFile := range indexFiles {
		if !strings.HasSuffix(indexFile.Name(), filepath.Base(output)) {
			continue
		}
		inFile := filepath.Join(indexDir, indexFile.Name())
		in, err := os.Open(inFile)
		if err != nil {
			return err
		}
		io.Copy(outputFile, in)
		if !noCleanup {
			os.Remove(inFile)
		}
	}

	fmt.Printf("done!\n")
	return nil
}

func divisionOfLabor(target string, maxWorkers int) ([]Labor, error) {
	targetInfo, err := os.Stat(target)
	if os.IsNotExist(err) {
		return nil, err
	}

	targetFile, err := os.Open(target)
	if err != nil {
		return nil, err
	}
	defer targetFile.Close()

	chunkSize := int64(math.Ceil(float64(targetInfo.Size()) / float64(maxWorkers)))

	offsets := []Labor{}
	position := int64(0)
	for id := 0; id < maxWorkers-1; id++ {
		cursor := position + chunkSize
		targetFile.Seek(cursor, 0)
		for {
			buf := make([]byte, 1)
			_, err := targetFile.ReadAt(buf, cursor)
			if err != nil {
				panic(err)
			}
			if buf[0] == '\n' {
				break
			}
			cursor--
		}
		offsets = append(offsets, Labor{
			Start: position,
			Stop:  cursor,
		})
		position = cursor + 1
	}

	lastCursor := int64(0)
	if 1 <= len(offsets) {
		lastCursor = offsets[len(offsets)-1].Stop + 1
	}
	offsets = append(offsets, Labor{
		Start: lastCursor,
		Stop:  targetInfo.Size(),
	})

	return offsets, nil
}

// Start - Start indexer
func Start(target, output, key string, maxWorkers uint, tempDir string, noCleanup bool) error {

	if maxWorkers < 1 {
		maxWorkers = 1
	}

	offsets, err := divisionOfLabor(target, int(maxWorkers))
	if err != nil {
		return err
	}

	indexDir := filepath.Join(tempDir, ".indexes")
	err = os.MkdirAll(indexDir, 0700)
	if err != nil {
		return err
	}
	defer func() {
		if !noCleanup {
			os.RemoveAll(indexDir)
		}
	}()

	wg := sync.WaitGroup{}
	workers := []*Worker{}
	for id := 0; id < int(maxWorkers); id++ {
		wg.Add(1)
		outputPath := filepath.Join(indexDir, fmt.Sprintf("%d_%s", id, filepath.Base(output)))
		worker := &Worker{
			ID:         id,
			Wg:         &wg,
			TargetPath: target,
			OutputPath: outputPath,
			Labor:      offsets[id],
		}
		worker.start(key)
		workers = append(workers, worker)
	}
	wg.Wait()
	return mergeIndexes(output, indexDir, noCleanup)
}