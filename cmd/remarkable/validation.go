package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/alexgorbatchev/remarkable-cli/internal/doc"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Invocation checks that need no cloud or file access run while Cobra parses
// flags and validates positional arguments. Cobra reports those failures with
// the command's usage screen; silenceUsageOnRun only quiets errors raised after
// RunE starts.

const pairingCodeLength = 8

// checkedValue is a flag value that parse validates as pflag reads the command
// line, so pflag rejects a bad value before the command runs.
type checkedValue[T any] struct {
	target *T
	typ    string
	parse  func(string) (T, error)
}

func (v *checkedValue[T]) String() string { return fmt.Sprint(*v.target) }
func (v *checkedValue[T]) Type() string   { return v.typ }

func (v *checkedValue[T]) Set(s string) error {
	value, err := v.parse(s)
	if err != nil {
		return err
	}
	*v.target = value
	return nil
}

// checkedString stores value in target and returns a string flag value that
// parse validates and may normalize.
func checkedString(target *string, value string, parse func(string) (string, error)) pflag.Value {
	*target = value
	return &checkedValue[string]{target: target, typ: "string", parse: parse}
}

// validString adapts a validator that does not normalize its input.
func validString(validate func(string) error) func(string) (string, error) {
	return func(s string) (string, error) {
		return s, validate(s)
	}
}

func requireNonEmptyPath(path string) error {
	if path == "" {
		return errors.New("path must not be empty")
	}
	return nil
}

// pageIndex stores value in target and returns an int flag value that rejects
// negative page indexes, which no document has.
func pageIndex(target *int, value int) pflag.Value {
	*target = value
	return &checkedValue[int]{target: target, typ: "int", parse: parsePageIndex}
}

func parsePageIndex(s string) (int, error) {
	// Base 0 accepts the same integer syntax as pflag's own int flags.
	index, err := strconv.ParseInt(s, 0, strconv.IntSize)
	if err != nil {
		return 0, err
	}
	if index < 0 {
		return 0, errors.New("page index must be 0 or greater")
	}
	return int(index), nil
}

func pairingCodeArg(_ *cobra.Command, args []string) error {
	if n := len(strings.TrimSpace(args[0])); n != pairingCodeLength {
		return fmt.Errorf("pairing code must be exactly %d characters (got %d)", pairingCodeLength, n)
	}
	return nil
}

func nonEmptyPathArg(_ *cobra.Command, args []string) error {
	return requireNonEmptyPath(args[0])
}

func settingsIdentityArgs(_ *cobra.Command, args []string) error {
	return doc.ValidateSettingsIdentities(args[0], args[1])
}

func importDestinationArg(_ *cobra.Command, args []string) error {
	return doc.ValidateImportDestination(args[0])
}

func searchQueryArgs(_ *cobra.Command, args []string) error {
	return doc.ValidateSearchQuery(args[1])
}
