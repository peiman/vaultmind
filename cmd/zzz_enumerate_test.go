package cmd

import (
	"fmt"
	"testing"

	"github.com/spf13/cobra"
)

func TestZZZEnumerateAllCommands(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			hidden := ""
			if sub.Hidden {
				hidden = "\tHIDDEN"
			}
			runnable := ""
			if sub.Runnable() {
				runnable = "\tRUNNABLE"
			}
			fmt.Printf("ENUM\t%s%s%s\n", sub.CommandPath(), hidden, runnable)
			walk(sub)
		}
	}
	walk(RootCmd)
}
