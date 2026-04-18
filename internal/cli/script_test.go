package cli

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/adrg/xdg"
	"github.com/rogpeppe/go-internal/testscript"
)

// TestMain registers the kcm binary as a testscript-invocable command and
// runs the usual Go test machinery on top. Scripts under testdata/script/*.txt
// can then call `kcm ...` as if it were a real binary — the call is serviced
// in-process by NewRootCmd, which is faster than building and spawning the
// real binary while still exercising the full command tree.
func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"kcm": kcmTestMain,
	})
}

func kcmTestMain() {
	// Honor XDG_CONFIG_HOME that testscript sets per-test so state is isolated.
	xdg.Reload()

	root := NewRootCmd()
	root.SetArgs(os.Args[1:])
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	if err := root.ExecuteContext(context.Background()); err != nil {
		// Root has SilenceErrors=true so we surface the error ourselves on
		// stderr — testscript assertions rely on it being there.
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata/script",
		Setup: func(env *testscript.Env) error {
			// Each script gets its own state dir under $WORK.
			env.Setenv("XDG_CONFIG_HOME", env.WorkDir+"/.xdg")
			return nil
		},
	})
}
