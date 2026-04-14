package waf

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/rtfpessoa/clitr/internal/log"
	"go.uber.org/zap"
)

const (
	loginURL      = "https://app.traderepublic.com/login"
	wafCookieName = "aws-waf-token"
	defaultTimeout = 30 * time.Second
	pollInterval   = 500 * time.Millisecond
)

// FetchToken launches a headless browser, navigates to the Trade Republic login page,
// and waits for the AWS WAF challenge to set the aws-waf-token cookie.
func FetchToken(ctx context.Context) (string, error) {
	return FetchTokenFromURL(ctx, loginURL)
}

// FetchTokenFromURL is the testable core: it navigates to the given URL and polls
// for the aws-waf-token cookie. Exported for testing with a local server.
func FetchTokenFromURL(ctx context.Context, targetURL string) (string, error) {
	log.Info("Acquiring AWS WAF token via headless browser")

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-setuid-sandbox", true),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	taskCtx, timeoutCancel := context.WithTimeout(taskCtx, defaultTimeout)
	defer timeoutCancel()

	if err := chromedp.Run(taskCtx, chromedp.Navigate(targetURL)); err != nil {
		return "", fmt.Errorf("failed to navigate to login page: %w", err)
	}

	log.Debug("Navigated to login page, polling for WAF token cookie")

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-taskCtx.Done():
			return "", fmt.Errorf("timed out waiting for AWS WAF token: %w", taskCtx.Err())
		case <-ticker.C:
			var cookies []*network.Cookie
			if err := chromedp.Run(taskCtx, chromedp.ActionFunc(func(ctx context.Context) error {
				var err error
				cookies, err = network.GetCookies().Do(ctx)
				return err
			})); err != nil {
				log.Debug("Failed to read cookies, retrying", zap.Error(err))
				continue
			}

			for _, c := range cookies {
				if c.Name == wafCookieName {
					log.Info("AWS WAF token acquired successfully")
					return c.Value, nil
				}
			}
		}
	}
}
