package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"model-check/internal/auth"
	"model-check/internal/store"
)

const usage = `Usage:
  modelsexam                                  serve the web app
  modelsexam user reset-password <username>   set a new random password, sign the
                                              user out everywhere and delete all of
                                              that user's saved keys and schedules
  modelsexam credentials purge-all --yes      delete every saved key and schedule
                                              (incident response)`

// runCommand handles the operator subcommands. There is no self-service
// password reset: an operator resets it, and the user's saved keys go with it.
func runCommand(st *store.Store, args []string) int {
	ctx := context.Background()
	switch {
	case len(args) == 3 && args[0] == "user" && args[1] == "reset-password":
		u, err := st.UserByName(ctx, args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "user %q not found\n", args[2])
			return 1
		}
		password, err := auth.RandomPassword()
		if err == nil {
			var hash string
			if hash, err = auth.HashPassword(password); err == nil {
				err = st.SetPassword(ctx, u.ID, hash, "")
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "reset password: %v\n", err)
			return 1
		}
		n, err := st.DeleteUserCredentials(ctx, u.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "password was reset, but deleting saved keys failed: %v\n", err)
			return 1
		}
		fmt.Printf("New password for %s: %s\nAll sessions ended; %d saved keys and their schedules deleted.\nAsk the user to change this password after signing in.\n", u.Username, password, n)
		return 0
	case len(args) >= 2 && args[0] == "credentials" && args[1] == "purge-all":
		if len(args) != 3 || args[2] != "--yes" {
			fmt.Fprintln(os.Stderr, "This deletes every saved key and schedule. Re-run with --yes to confirm.")
			return 2
		}
		n, err := st.DeleteAllCredentials(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "purge: %v\n", err)
			return 1
		}
		fmt.Printf("Deleted %d saved keys and all schedules.\n", n)
		return 0
	case len(args) == 1 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help"):
		fmt.Println(usage)
		return 0
	}
	fmt.Fprintln(os.Stderr, errors.New("unknown command: "+strings.Join(args, " ")))
	fmt.Fprintln(os.Stderr, usage)
	return 2
}
