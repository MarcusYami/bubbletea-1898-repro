// The repro from the issue: huh's Form.Run with one Select; ctrl-c while the menu is up.
package main

import (
	"fmt"
	"os"

	"charm.land/huh/v2"
)

func main() {
	var pick string
	f := huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Title("What now?").
		Options(huh.NewOption("Build", "build"), huh.NewOption("Test", "test"),
			huh.NewOption("Ship", "ship")).Value(&pick)))
	if err := f.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
