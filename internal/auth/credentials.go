package auth

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

func SetCredentialsInteractive(dataDir string) error {
	auth, err := New(dataDir)
	if err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Username: ")
	username, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read username: %w", err)
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}

	fmt.Print("Password: ")
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	fmt.Println()
	password := string(passwordBytes)
	if password == "" {
		return fmt.Errorf("password cannot be empty")
	}

	fmt.Print("Confirm password: ")
	confirmBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("read confirm: %w", err)
	}
	fmt.Println()

	if string(confirmBytes) != password {
		return fmt.Errorf("passwords do not match")
	}

	if err := auth.SetCredentials(username, password); err != nil {
		return err
	}

	fmt.Printf("Credentials saved to %s\n", auth.authPath)
	return nil
}
