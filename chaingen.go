package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

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
	flags.BoolVar(&options.ErrOnConflict, "err-on-conflict", true, "Whether to return error if method naming conflict is encountered")
	flags.StringVar(&options.StructTag, "struct-tag", "chaingen", "Sets struct tag name to use")
	flags.StringVar(&options.BuildTag, "build-tag", "chaingen", "Sets go build tag name that is used to ignore generated files while analyzing code")
}

func main() {
	err := flags.Parse(os.Args[1:])
	if err != nil {
		log.Println(err.Error())
		flags.Usage()
		os.Exit(2)
	}
	options.Src, err = filepath.Abs(options.Src)
	if err != nil {
		fmt.Println(err.Error())
		flags.Usage()
		os.Exit(2)
	}
	err = chaingen.New(*options).Generate()
	if err != nil {
		fmt.Println(err.Error())
		flags.Usage()
		os.Exit(1)
	}
}
