package client

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/rtfpessoa/clitr/internal/log"
)

func (c *Client) AuthenticateClient() error {
	log.Info("Connecting to Trade Republic")
	if c.saveCookies && c.resumeWebSession() {
		log.Info("Resumed existing session from saved cookies")
		return nil
	}
	return c.loginInteractively()
}

func (c *Client) loginInteractively() error {
	log.Info("Initiating login")
	input := bufio.NewReader(c.stdinReader)
	phoneNo, pin, err := collectPinWithReader(input, c.phoneNo)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}
	if c.phoneNo == "" {
		c.phoneNo = phoneNo
	}
	challenge, err := c.InitiateWebLoginV2(phoneNo, pin)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}
	return finishInteractiveLogin(c, input, challenge)
}

func finishInteractiveLogin(c *Client, input *bufio.Reader, challenge LoginChallenge) error {
	code := ""
	if challenge.RequiresAuthenticator {
		fmt.Print("Authenticator code: ")
		codeInput, err := input.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read authenticator code: %w", err)
		}
		code = strings.TrimSpace(codeInput)
	} else {
		fmt.Printf("Approve the login in your Trade Republic app (within %d seconds).\n", challenge.Countdown)
	}
	if err := c.CompleteWebLoginV2(context.Background(), code); err != nil {
		return fmt.Errorf("login approval failed: %w", err)
	}
	log.Info("Login successful")
	return nil
}
