package web

import (
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
)

// TokenCommand is the "token" subcommand of a service: it creates and removes
// the bearer tokens in the service's token file and lists their names.
//
//	<app> token add <name> [-config <file>] [-dry-run]
//	<app> token remove <name> [-config <file>] [-dry-run]
//	<app> token list [-config <file>]
//
// add writes the hash to the token file first and prints the token only once
// that succeeded, on stdout and nowhere else; explanations go to stderr. With
// -dry-run it prints the token and the line for the file without writing
// anything. A token file that does not exist yet is created with mode 0640
// and the group of the config file, so a service that reads its config
// through its group can read the tokens too. The owner stays whoever ran the
// command - root, normally - and is never taken from the config file: in an
// old layout that belongs to the service user, who could then write the
// token file and add their own token.
//
// The service hangs it into its main before parsing its own flags:
//
//	if len(os.Args) > 1 && os.Args[1] == "token" {
//	    os.Exit(web.TokenCommand{
//	        App:           "tadl",
//	        DefaultConfig: "/opt/tadl/etc/config.yaml",
//	        TokenFile:     app.TokenFile, // reads webserver.tokenFile
//	    }.Run(os.Args[2:]))
//	}
type TokenCommand struct {
	// App is the name of the service, used in messages.
	App string
	// DefaultConfig is the config file used when -config is not given.
	DefaultConfig string
	// TokenFile returns the path of the token file configured in the config
	// file at configPath, or "" if none is configured.
	TokenFile func(configPath string) (string, error)
	// Apply tells the user how a change takes effect; "Restart <App> to
	// apply." if empty. A service that rereads its token file on SIGHUP says
	// so here, e.g. "Apply with: systemctl reload tadl".
	Apply string
	// Stdout receives the token and the list of names; os.Stdout if nil.
	Stdout io.Writer
	// Stderr receives everything else; os.Stderr if nil.
	Stderr io.Writer
}

// Run executes the subcommand with args, the arguments after "token", and
// returns the exit code: 0 on success, 1 on failure, 2 on a usage error.
func (c TokenCommand) Run(args []string) int {
	if c.Stdout == nil {
		c.Stdout = os.Stdout
	}
	if c.Stderr == nil {
		c.Stderr = os.Stderr
	}
	stdout, stderr := &printer{w: c.Stdout}, &printer{w: c.Stderr}

	flags := flag.NewFlagSet(c.App+" token", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	configPath := flags.String("config", c.DefaultConfig, "config file that names the token file")
	dryRun := flags.Bool("dry-run", false, "only print what would be done, change nothing")
	flags.Usage = func() {
		stderr.printf("usage:\n"+
			"  %[1]s token add <name> [-config <file>] [-dry-run]\n"+
			"  %[1]s token remove <name> [-config <file>] [-dry-run]\n"+
			"  %[1]s token list [-config <file>]\n\n", c.App)
		flags.PrintDefaults()
	}

	// Flags may come before, between and after the positional arguments.
	var pos []string
	for rest := args; ; {
		if err := flags.Parse(rest); err != nil {
			return 2
		}
		rest = flags.Args()
		if len(rest) == 0 {
			break
		}
		pos, rest = append(pos, rest[0]), rest[1:]
	}

	var err error
	switch {
	case len(pos) == 2 && pos[0] == "add":
		err = c.add(stdout, stderr, *configPath, pos[1], *dryRun)
	case len(pos) == 2 && pos[0] == "remove":
		err = c.remove(stderr, *configPath, pos[1], *dryRun)
	case len(pos) == 1 && pos[0] == "list":
		err = c.list(stdout, stderr, *configPath)
	default:
		flags.Usage()
		return 2
	}

	if err != nil {
		stderr.printf("%s token: %v\n", c.App, err)
		return 1
	}
	return 0
}

func (c TokenCommand) add(stdout, stderr *printer, configPath, name string, dryRun bool) error {
	if !ValidTokenName(name) {
		return fmt.Errorf("%w %q: 1 to 64 letters, digits, '.', '_' or '-'", ErrInvalidTokenName, name)
	}

	token, err := NewToken()
	if err != nil {
		return err
	}
	hash := HashToken(token)

	if dryRun {
		stdout.printf("%s\n", token)
		if stdout.err != nil {
			return stdout.err
		}
		stderr.printf("\nDry run, nothing written. Add this line to the token file:\n  %s %s\n%s\n"+
			"The token above is shown only now; store it with the caller.\n",
			name, hex.EncodeToString(hash[:]), c.apply())
		return nil
	}

	path, err := c.tokenFile(configPath)
	if err != nil {
		return fmt.Errorf("%w; use -dry-run to only print the line", err)
	}

	_, statErr := os.Stat(path)
	created := errors.Is(statErr, fs.ErrNotExist)

	if err := AddTokenToFile(path, name, hash); err != nil {
		return err
	}

	if created {
		if err := chgrpLike(path, configPath); err != nil {
			stderr.printf("warning: created %s, but could not give it the group of %s: %v\n", path, configPath, err)
		}
	}

	stdout.printf("%s\n", token)
	if stdout.err != nil {
		return fmt.Errorf("%q was added to %s, but the token could not be printed (%w); "+
			"remove it with: %s token remove %s", name, path, stdout.err, c.App, name)
	}
	stderr.printf("\nAdded %q to %s. %s\n"+
		"The token above is shown only now; store it with the caller.\n", name, path, c.apply())
	return nil
}

func (c TokenCommand) remove(stderr *printer, configPath, name string, dryRun bool) error {
	path, err := c.tokenFile(configPath)
	if err != nil {
		return err
	}

	if dryRun {
		tokens, err := ReadTokenFile(path)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(tokens, func(t Token) bool { return t.Name == name }) {
			return fmt.Errorf("%w: %q", ErrTokenNotFound, name)
		}
		stderr.printf("Dry run, nothing written. Would remove %q from %s.\n", name, path)
		return nil
	}

	if err := RemoveTokenFromFile(path, name); err != nil {
		return err
	}
	stderr.printf("Removed %q from %s. %s\n", name, path, c.apply())
	return nil
}

func (c TokenCommand) list(stdout, stderr *printer, configPath string) error {
	path, err := c.tokenFile(configPath)
	if err != nil {
		return err
	}

	tokens, err := ReadTokenFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		stderr.printf("No token file yet: %s\n", path)
		return nil
	}
	if err != nil {
		return err
	}

	for _, t := range tokens {
		stdout.printf("%s\n", t.Name)
	}
	if stdout.err != nil {
		return stdout.err
	}
	return nil
}

// apply returns the hint how a change takes effect.
func (c TokenCommand) apply() string {
	if c.Apply != "" {
		return c.Apply
	}
	return "Restart " + c.App + " to apply."
}

// tokenFile asks the service for the token file named in its config.
func (c TokenCommand) tokenFile(configPath string) (string, error) {
	if c.TokenFile == nil {
		return "", errors.New("the service does not say where its token file is")
	}
	path, err := c.TokenFile(configPath)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", fmt.Errorf("%s names no token file (webserver.tokenFile)", configPath)
	}
	return path, nil
}

// printer writes to w and keeps the first error, so a caller checks once
// instead of after every line.
type printer struct {
	w   io.Writer
	err error
}

func (p *printer) printf(format string, args ...any) {
	if p.err == nil {
		_, p.err = fmt.Fprintf(p.w, format, args...)
	}
}

// chgrpLike gives path the group of ref; the owner is left alone.
func chgrpLike(path, ref string) error {
	info, err := os.Stat(ref)
	if err != nil {
		return err
	}
	_, gid, ok := fileOwner(info)
	if !ok || gid == os.Getegid() {
		return nil
	}
	return os.Chown(path, -1, gid)
}
