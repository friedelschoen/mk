package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/spf13/pflag"
	"golang.org/x/term"
)

var (
	// True if messages should be printed with fancy colors.
	// By default, if the output stream is not the terminal, colors are disabled.
	color bool

	// Default shell to use if none specified via $shell.
	defaultShell string

	// Do not drop shell arguments when calling with no further arguments
	// This works around `sh -c commands...` being a thing, but allows the `rc -v commands...` argument-less pflags
	dontDropArgs bool

	// True if we are ignoring timestamps and rebuilding everything.
	rebuildall bool = false

	// Set of targets for which we are forcing rebuild
	rebuildtargets map[string]bool = make(map[string]bool)

	// Lock on standard out, messages don't get interleaved too much.
	mkMsgMutex sync.Mutex

	// Limit the number of recipes executed simultaneously.
	subprocsAllowed int

	// Current subprocesses being executed
	subprocsRunning int

	// Wakeup on a free subprocess slot.
	subprocsRunningCond *sync.Cond = sync.NewCond(&sync.Mutex{})

	// Prevent more than one recipe at a time from trying to take over
	exclusiveSubproc = sync.Mutex{}

	// The maximum number of times an rule may be applied.
	// This limits recursion of both meta- and non-meta-rules!
	// Maybe, this shouldn't affect meta-rules?!
	maxRuleCnt int = 1

	// delimiter for lists in environment, defaults to '\x01' when defaultShell==rc otherwise ':'
	shellDelimiter string
)

// Wait until there is an available subprocess slot.
func reserveSubproc() {
	subprocsRunningCond.L.Lock()
	for subprocsRunning >= subprocsAllowed {
		subprocsRunningCond.Wait()
	}
	subprocsRunning++
	subprocsRunningCond.L.Unlock()
}

// Free up another subprocess to run.
func finishSubproc() {
	subprocsRunningCond.L.Lock()
	subprocsRunning--
	subprocsRunningCond.Signal()
	subprocsRunningCond.L.Unlock()
}

// Make everyone wait while we
func reserveExclusiveSubproc() {
	exclusiveSubproc.Lock()
	// Wait until everything is done running
	stolenSubprocs := 0
	subprocsRunningCond.L.Lock()
	stolenSubprocs = subprocsAllowed - subprocsRunning
	subprocsRunning = subprocsAllowed
	for stolenSubprocs < subprocsAllowed {
		subprocsRunningCond.Wait()
		stolenSubprocs += subprocsAllowed - subprocsRunning
		subprocsRunning = subprocsAllowed
	}
}

func finishExclusiveSubproc() {
	subprocsRunning = 0
	subprocsRunningCond.Broadcast()
	subprocsRunningCond.L.Unlock()
	exclusiveSubproc.Unlock()
}

// Ansi color codes.
const (
	ansiTermDefault   = "\033[0m"
	ansiTermBlack     = "\033[30m"
	ansiTermRed       = "\033[31m"
	ansiTermGreen     = "\033[32m"
	ansiTermYellow    = "\033[33m"
	ansiTermBlue      = "\033[34m"
	ansiTermMagenta   = "\033[35m"
	ansiTermBright    = "\033[1m"
	ansiTermUnderline = "\033[4m"
)

// Build a node's prereqs. Block until completed.
func mkNodePrereqs(g *graph, u *node, e *edge, prereqs []*node, dryrun bool,
	required bool) nodeStatus {
	prereqstat := make(chan nodeStatus)
	pending := 0

	// build prereqs that need building
	for i := range prereqs {
		prereqs[i].mutex.Lock()
		switch prereqs[i].status {
		case nodeStatusReady, nodeStatusNop:
			go mkNode(g, prereqs[i], dryrun, required)
			fallthrough
		case nodeStatusStarted:
			prereqs[i].listeners = append(prereqs[i].listeners, prereqstat)
			pending++
		}
		prereqs[i].mutex.Unlock()
	}

	// wait until all the prereqs are built
	status := nodeStatusDone
	for pending > 0 {
		s := <-prereqstat
		pending--
		if s == nodeStatusFailed {
			status = nodeStatusFailed
		}
	}
	return status
}

// Build a target in the graph.
//
// This selects an appropriate rule (edge) and builds all prerequisites
// concurrently.
//
// Args:
//
//	g: Graph in which the node lives.
//	u: Node to (possibly) build.
//	dryrun: Don't actually build anything, just pretend.
//	required: Avoid building this node, unless its prereqs are out of date.
func mkNode(g *graph, u *node, dryrun bool, required bool) {
	// try to claim on this node
	u.mutex.Lock()
	if u.status != nodeStatusReady && u.status != nodeStatusNop {
		u.mutex.Unlock()
		return
	} else {
		u.status = nodeStatusStarted
	}
	u.mutex.Unlock()

	// when finished, notify the listeners
	finalstatus := nodeStatusDone
	defer func() {
		u.mutex.Lock()
		u.status = finalstatus
		for i := range u.listeners {
			u.listeners[i] <- u.status
		}
		u.listeners = u.listeners[0:0]
		u.mutex.Unlock()
	}()

	// there's no rules.
	if len(u.prereqs) == 0 {
		if !(u.r != nil && u.r.attributes.virtual) && !u.exists {
			wd, _ := os.Getwd()
			mkError(fmt.Sprintf("don't know how to make %s in %s\n", u.name, wd))
		}
		finalstatus = nodeStatusNop
		return
	}

	// there should otherwise be exactly one edge with an associated rule
	prereqs := make([]*node, 0)
	var e *edge = nil
	for i := range u.prereqs {
		if u.prereqs[i].r != nil {
			e = u.prereqs[i]
		}
		if u.prereqs[i].v != nil {
			prereqs = append(prereqs, u.prereqs[i].v)
		}
	}

	// this should have been caught during graph building
	if e == nil {
		wd, _ := os.Getwd()
		mkError(fmt.Sprintf("don't know how to make %s in %s", u.name, wd))
	}

	prereqsRequired := required && (e.r.attributes.virtual || !u.exists)
	mkNodePrereqs(g, u, e, prereqs, dryrun, prereqsRequired)

	uptodate := true
	if !e.r.attributes.virtual {
		u.updateTimestamp()
		if !u.exists && required {
			uptodate = false
		} else if u.exists || required {
			for i := range prereqs {
				if u.t.Before(prereqs[i].t) || prereqs[i].status == nodeStatusDone {
					uptodate = false
				}
			}
		} else if required {
			uptodate = false
		}
	} else {
		uptodate = false
	}

	_, isrebuildtarget := rebuildtargets[u.name]
	if isrebuildtarget || rebuildall {
		uptodate = false
	}

	// make another pass on the prereqs, since we know we need them now
	if !uptodate {
		mkNodePrereqs(g, u, e, prereqs, dryrun, true)
	}

	// execute the recipe, unless the prereqs failed
	if !uptodate && finalstatus != nodeStatusFailed && len(e.r.recipe) > 0 {
		if e.r.attributes.exclusive {
			reserveExclusiveSubproc()
		} else {
			reserveSubproc()
		}

		if !dorecipe(u.name, u, e, dryrun) {
			finalstatus = nodeStatusFailed
		}
		u.updateTimestamp()

		if e.r.attributes.exclusive {
			finishExclusiveSubproc()
		} else {
			finishSubproc()
		}
	} else if finalstatus != nodeStatusFailed {
		finalstatus = nodeStatusNop
	}
}

func mkError(msg string) {
	mkPrintError(msg)
	os.Exit(1)
}

func mkPrintError(msg string) {
	if color {
		os.Stderr.WriteString(ansiTermRed)
	}
	fmt.Fprintf(os.Stderr, "error: %s\n", msg)
	if color {
		os.Stderr.WriteString(ansiTermDefault)
	}
}

func mkPrintRecipe(target string, recipe string, quiet bool) {
	mkMsgMutex.Lock()
	if !color {
		fmt.Printf("%s: ", target)
	} else {
		fmt.Printf("%s%s%s → %s",
			ansiTermBlue+ansiTermBright+ansiTermUnderline, target,
			ansiTermDefault, ansiTermBlue)
	}
	if quiet {
		if !color {
			fmt.Println("...")
		} else {
			fmt.Println("…")
		}
	} else {
		printIndented(os.Stdout, recipe, len(target)+3)
		if len(recipe) == 0 {
			os.Stdout.WriteString("\n")
		}
	}
	if color {
		os.Stdout.WriteString(ansiTermDefault)
	}
	mkMsgMutex.Unlock()
}

func main() {
	var directory string
	var mkfilepath string
	var interactive bool
	var dryrun bool
	var shallowrebuild bool
	var quiet bool
	var shellOS string

	pflag.StringVarP(&directory, "directory", "C", "", "directory to change in to")
	pflag.StringVarP(&mkfilepath, "file", "f", "mkfile", "use the given file as mkfile")
	pflag.BoolVarP(&dryrun, "dry-run", "n", false, "print commands without actually executing")
	pflag.BoolVar(&shallowrebuild, "force-target", false, "force building of just targets")
	pflag.BoolVar(&rebuildall, "force-all", false, "force building of all dependencies")
	pflag.IntVarP(&subprocsAllowed, "jobs", "j", runtime.NumCPU(), "maximum number of jobs to execute in parallel")
	pflag.IntVarP(&maxRuleCnt, "depth", "d", 1, "maximum number of times a specific rule can be applied (recursion)")
	pflag.BoolVarP(&interactive, "interactive", "i", false, "ask before executing rules")
	pflag.BoolVarP(&quiet, "quiet", "q", false, "don't print recipes before executing them")
	pflag.BoolVar(&color, "color", term.IsTerminal(int(os.Stdout.Fd())), "turn color on/off")
	pflag.StringVar(&defaultShell, "shell", "sh -c", "default shell to use if none are specified via $shell")
	pflag.BoolVar(&dontDropArgs, "drop-shell-arg", false, "don't drop shell arguments when no further arguments are specified")
	pflag.StringVar(&shellOS, "shell-delimiter", runtime.GOOS, "delimiter in a list in the environment")
	pflag.Parse()

	switch shellOS {
	case "plan9":
		shellDelimiter = "\x01"
	default:
		shellDelimiter = ":"
	}

	if directory != "" {
		err := os.Chdir(directory)
		if err != nil {
			mkError(fmt.Sprintf("changing directory to `%s' failed", directory))
		}
	}

	input, err := os.Open(mkfilepath)
	if err != nil {
		mkError("no mkfile found")
	}
	defer input.Close()

	abspath, err := filepath.Abs(mkfilepath)
	if err != nil {
		mkError("unable to find mkfile's absolute path")
	}

	env := make(map[string][]string)
	for _, elem := range os.Environ() {
		vals := strings.SplitN(elem, "=", 2)
		env[vals[0]] = append(env[vals[0]], vals[1])
	}

	rs := parse(input, mkfilepath, abspath, env)
	if quiet {
		for i := range rs.rules {
			rs.rules[i].attributes.quiet = true
		}
	}

	targets := pflag.Args()

	// build the first non-meta rule in the makefile, if none are given explicitly
	if len(targets) == 0 {
		for i := range rs.rules {
			if !rs.rules[i].ismeta {
				for j := range rs.rules[i].targets {
					targets = append(targets, rs.rules[i].targets[j].spat)
				}
				break
			}
		}
	}

	if len(targets) == 0 {
		fmt.Println("mk: nothing to mk")
		return
	}

	if shallowrebuild {
		for i := range targets {
			rebuildtargets[targets[i]] = true
		}
	}

	// Create a dummy virtual rule that depends on every target
	root := rule{}
	root.targets = []pattern{{false, "", nil}}
	root.attributes = attribSet{false, false, false, false, false, false, false, true, false}
	root.prereqs = targets
	rs.add(root)

	// Keep a global reference to the total state of mk variables.
	GlobalMkState = rs.vars

	if interactive {
		g := buildgraph(rs, "")
		mkNode(g, g.root, true, true)
		fmt.Print("Proceed? ")
		in := bufio.NewReader(os.Stdin)
		for {
			c, _, err := in.ReadRune()
			if err != nil {
				return
			} else if strings.ContainsRune(" \n\t\r", c) {
				continue
			} else if c == 'y' {
				break
			} else {
				return
			}
		}
	}

	g := buildgraph(rs, "")
	mkNode(g, g.root, dryrun, true)
}

var GlobalMkState map[string][]string
