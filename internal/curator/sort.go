package curator

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
*/

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/moloch--/leakdb/pkg/sorter"
	"github.com/spf13/cobra"
)

const (
	maxGoRoutines = 10000
)

var sortCmd = &cobra.Command{
	Use:   "sort",
	Short: "Sort an index file",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		index, err := cmd.Flags().GetString(indexFlagStr)
		if err != nil {
			fmt.Printf("Failed to parse --%s flag: %s\n", indexFlagStr, err)
			return
		}
		output, err := cmd.Flags().GetString(outputFlagStr)
		if err != nil {
			fmt.Printf("Failed to parse --%s flag: %s\n", outputFlagStr, err)
			return
		}
		maxMemory, err := cmd.Flags().GetUint(maxMemoryFlagStr)
		noCleanup, err := cmd.Flags().GetBool(noCleanupFlagStr)
		if err != nil {
			fmt.Printf("Failed to parse --%s flag: %s\n", noCleanupFlagStr, err)
			return
		}
		tempDir, err := cmd.Flags().GetString(tempDirFlagStr)
		if err != nil {
			fmt.Printf("Failed to parse --%s flag: %s\n", tempDirFlagStr, err)
			return
		}
		if tempDir == "" {
			cwd, _ := os.Getwd()
			tempDir, err = ioutil.TempDir(cwd, ".leakdb_")
			if err != nil {
				fmt.Printf("Temp error: %s\n", err)
				return
			}
		}
		if !noCleanup {
			defer os.RemoveAll(tempDir)
		}

		sorter.Start(index, output, int(maxMemory), maxGoRoutines, tempDir, noCleanup)

	},
}