package action

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/gopasspw/gopass/internal/audit"
	"github.com/gopasspw/gopass/internal/editor"
	"github.com/gopasspw/gopass/internal/out"
	"github.com/gopasspw/gopass/internal/store"
	"github.com/gopasspw/gopass/internal/store/leaf"
	"github.com/gopasspw/gopass/internal/store/secret"
	"github.com/gopasspw/gopass/internal/termio"
	"github.com/gopasspw/gopass/pkg/ctxutil"

	"github.com/urfave/cli/v2"
)

// Insert a string as content to a secret file
func (s *Action) Insert(c *cli.Context) error {
	ctx := ctxutil.WithGlobalFlags(c)
	echo := c.Bool("echo")
	multiline := c.Bool("multiline")
	force := c.Bool("force")
	append := c.Bool("append")

	args, kvps := parseArgs(c)
	name := args.Get(0)
	key := args.Get(1)

	if name == "" {
		return ExitError(ctx, ExitNoName, nil, "Usage: %s insert name", s.Name)
	}

	return s.insert(ctx, c, name, key, echo, multiline, force, append, kvps)
}

func (s *Action) insert(ctx context.Context, c *cli.Context, name, key string, echo, multiline, force, append bool, kvps map[string]string) error {
	// if force mode is requested we mock the recipient func to just return anything that goes in
	if force {
		ctx = leaf.WithRecipientFunc(ctx, func(ctx context.Context, msg string, rs []string) ([]string, error) {
			return rs, nil
		})
	}

	var content []byte

	// if content is piped to stdin, read and save it
	if ctxutil.IsStdin(ctx) {
		buf := &bytes.Buffer{}

		if written, err := io.Copy(buf, stdin); err != nil {
			return ExitError(ctx, ExitIO, err, "failed to copy after %d bytes: %s", written, err)
		}

		content = buf.Bytes()
	}

	// update to a single YAML entry
	if key != "" {
		return s.insertYAML(ctx, name, key, content, kvps)
	}

	if ctxutil.IsStdin(ctx) {
		if !force && !append && s.Store.Exists(ctx, name) {
			return ExitError(ctx, ExitAborted, nil, "not overwriting your current secret")
		}
		return s.insertStdin(ctx, name, content, append)
	}

	// don't check if it's force anyway
	if !force && s.Store.Exists(ctx, name) && !termio.AskForConfirmation(ctx, fmt.Sprintf("An entry already exists for %s. Overwrite it?", name)) {
		return ExitError(ctx, ExitAborted, nil, "not overwriting your current secret")
	}

	// if multi-line input is requested start an editor
	if multiline && ctxutil.IsInteractive(ctx) {
		return s.insertMultiline(ctx, c, name)
	}

	// if echo mode is requested use a simple string input function
	if echo {
		ctx = termio.WithPassPromptFunc(ctx, func(ctx context.Context, prompt string) (string, error) {
			return termio.AskForString(ctx, prompt, "")
		})
	}

	pw, err := termio.AskForPassword(ctx, name)
	if err != nil {
		return ExitError(ctx, ExitIO, err, "failed to ask for password: %s", err)
	}

	return s.insertSingle(ctx, name, pw, kvps)
}

func (s *Action) insertStdin(ctx context.Context, name string, content []byte, appendTo bool) error {
	if appendTo && s.Store.Exists(ctx, name) {
		sec, err := s.Store.Get(ctx, name)
		if err != nil {
			return ExitError(ctx, ExitDecrypt, err, "failed to decrypt existing secret: %s", err)
		}
		buf, err := sec.Bytes()
		if err != nil {
			return ExitError(ctx, ExitDecrypt, err, "failed to decode existing secret: %s", err)
		}
		content = append(buf, content...)
	}
	sec, err := secret.Parse(content)
	if err != nil {
		out.Error(ctx, "WARNING: Invalid YAML: %s", err)
	}
	if err := s.Store.Set(ctxutil.WithCommitMessage(ctx, "Read secret from STDIN"), name, sec); err != nil {
		return ExitError(ctx, ExitEncrypt, err, "failed to set '%s': %s", name, err)
	}
	return nil
}

func (s *Action) insertSingle(ctx context.Context, name, pw string, kvps map[string]string) error {
	var sec store.Secret
	if s.Store.Exists(ctx, name) {
		var err error
		sec, err = s.Store.Get(ctx, name)
		if err != nil {
			return ExitError(ctx, ExitDecrypt, err, "failed to decrypt existing secret: %s", err)
		}
	} else {
		sec = &secret.Secret{}

		if content, found := s.renderTemplate(ctx, name, []byte(pw)); found {
			nSec, err := secret.Parse(content)
			if err == nil {
				sec = nSec
			}
		}
	}
	setMetadata(sec, kvps)
	sec.SetPassword(pw)
	audit.Single(ctx, sec.Password())

	if err := s.Store.Set(ctxutil.WithCommitMessage(ctx, "Inserted user supplied password"), name, sec); err != nil {
		return ExitError(ctx, ExitEncrypt, err, "failed to write secret '%s': %s", name, err)
	}
	return nil
}

func (s *Action) insertYAML(ctx context.Context, name, key string, content []byte, kvps map[string]string) error {
	if ctxutil.IsInteractive(ctx) {
		pw, err := termio.AskForString(ctx, name+":"+key, "")
		if err != nil {
			return ExitError(ctx, ExitIO, err, "failed to ask for user input: %s", err)
		}
		content = []byte(pw)
	}

	var sec store.Secret
	if s.Store.Exists(ctx, name) {
		var err error
		sec, err = s.Store.Get(ctx, name)
		if err != nil {
			return ExitError(ctx, ExitEncrypt, err, "failed to set key '%s' of '%s': %s", key, name, err)
		}
	} else {
		sec = &secret.Secret{}
	}
	setMetadata(sec, kvps)
	if err := sec.SetValue(key, string(content)); err != nil {
		return ExitError(ctx, ExitEncrypt, err, "failed to set key '%s' of '%s': %s", key, name, err)
	}
	if err := s.Store.Set(ctxutil.WithCommitMessage(ctx, "Inserted YAML value from STDIN"), name, sec); err != nil {
		return ExitError(ctx, ExitEncrypt, err, "failed to set key '%s' of '%s': %s", key, name, err)
	}
	return nil
}

func (s *Action) insertMultiline(ctx context.Context, c *cli.Context, name string) error {
	buf := []byte{}
	if s.Store.Exists(ctx, name) {
		var err error
		sec, err := s.Store.Get(ctx, name)
		if err != nil {
			return ExitError(ctx, ExitDecrypt, err, "failed to decrypt existing secret: %s", err)
		}
		buf, err = sec.Bytes()
		if err != nil {
			return ExitError(ctx, ExitUnknown, err, "failed to encode secret: %s", err)
		}
	}
	ed := editor.Path(c)
	content, err := editor.Invoke(ctx, ed, buf)
	if err != nil {
		return ExitError(ctx, ExitUnknown, err, "failed to start editor: %s", err)
	}
	sec, err := secret.Parse(content)
	if err != nil {
		out.Error(ctx, "WARNING: Invalid YAML: %s", err)
	}
	if err := s.Store.Set(ctxutil.WithCommitMessage(ctx, fmt.Sprintf("Inserted user supplied password with %s", ed)), name, sec); err != nil {
		return ExitError(ctx, ExitEncrypt, err, "failed to store secret '%s': %s", name, err)
	}
	return nil
}
