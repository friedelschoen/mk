package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

var (
	alwaysBuild = flag.Bool("a", false, "Assume all targets to be out of date.")
	//    -a      Assume all targets to be out of date.  Thus, everything is
	//            updated.
	changeDir = flag.String("c", "", "Change `dir`ectory before executing.")
	//    -cdir   Change directory before executing.
	debugOutput = flag.Bool("d", false, "Produce debugging output.")
	//    -d[egp] Produce debugging output (p is for parsing, g for graph
	//            building, e for execution).
	expain = flag.Bool("e", false, "Explain why each target is made.")
	//    -e      Explain why each target is made.
	mkfile = flag.String("f", "mkfile", "Use `file` rather than 'mkfile'.")
	//    -ffile  Use file rather than `mkfile`.
	forceIntermediate = flag.Bool("i", false, "Force any missing intermediate targets to be made.")
	//    -i      Force any missing intermediate targets to be made.
	recoverErrors = flag.Bool("k", false, "Do as much work as possible in the face of errors.")
	//    -k      Do as much work as possible in the face of errors.
	doNothing = flag.Bool("n", false, " Print, but do not execute, the commands needed to update the targets.")
	//    -n      Print, but do not execute, the commands needed to update the
	//            targets.
	includeDir = flag.Bool("r", false, "Search absolute includes in root and here.")
	//    -rdir1   Search absolute includes in root and here.
	noParallel = flag.Bool("s", false, "Make the command line arguments sequentially rather than in parallel.")
	//    -s      Make the command line arguments sequentially rather than in
	//            parallel.
	doNothingTouch = flag.Bool("t", false, "Touch (update the modified date of) file targets, without executing any recipes.")
	//    -t      Touch (update the modified date of) file targets, without
	//            executing any recipes.
	assumeNewTarget = flag.String("w", "", "Pretend the modify time for each target is the current time.")
	//    -wtarget1,target2,...
	//            Pretend the modify time for each target is the current time;
	//            useful in conjunction with -n to learn what updates would be
	//            triggered by modifying the targets.
	shell = flag.String("x", "sh", "Use `cmd` to execute recipes, must understand '-c <recipe>'.")
	//    -xcmd   Use shell to execute recipes, must understand `-c <recipe>`.
)

func main() {
	flag.Parse()

	if *changeDir != "" {
		os.Chdir(*changeDir)
	}

	file, err := os.Open(*mkfile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	defer file.Close()

	env := make(map[string]string)
	for _, pair := range os.Environ() {
		k, v, _ := strings.Cut(pair, "=")
		env[k] = v
	}

	parser := &Graph{
		vars: env,
	}

	if err := parser.parseFile(file, ".", "mkfile"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if flag.NArg() == 0 {
		r := parser.DefaultTarget()
		if r == nil {
			fmt.Fprintf(os.Stderr, "mk: nothing to mk\n")
			os.Exit(1)
		}

		err = parser.BuildRule(r, nil, "<default>", "", nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	for _, name := range flag.Args() {
		err := parser.Build(nil, name)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
