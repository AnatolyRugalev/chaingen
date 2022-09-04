package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/AnatolyRugalev/chaingen/pkg/chaingen"
)

var (
	flags   = flag.NewFlagSet("chaingen", flag.ExitOnError)
	options = &chaingen.Options{}
)

func init() {
	wd, _ := os.Getwd()
	flags.StringVar(&options.Src, "src", wd, "Builder package directory")
	flags.StringVar(&options.TypeName, "type", "", "Builder struct type name. If not set, all struct types will be considered")
	flags.BoolVar(&options.Recursive, "recursive", true, "Whether to recuresively generate code for nested builders")
	flags.StringVar(&options.FileSuffix, "file-suffix", ".chaingen.go", "Generated file suffix, including '.go'")
}

func main() {
	err := flags.Parse(os.Args[1:])
	if err != nil {
		panic(err)
	}
	err = chaingen.New(*options).Generate()
	if err != nil {
		fmt.Println(err.Error())
		flags.Usage()
		os.Exit(1)
	}
}
