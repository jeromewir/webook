package main

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/chromedp/chromedp"
)

const PageLogin = "login"
const PageReserve = "reserve"

var ErrDateInOlderThanOneMonthFuture = errors.New("date is more than 31 days in the future")

func login(ctx context.Context, email string, password string) error {
	return chromedp.Run(ctx,
		chromedp.Click(`//button[text()="Member log in"]`, chromedp.BySearch),
		raceItemsChromeFn(ctx, []BrowserSwitchAction{
			{
				Checker: func(ctx context.Context) {
					chromedp.WaitReady(`input[id="username"][value=""]`, chromedp.ByQuery).Do(ctx)
				},
				Action: func(ctx context.Context) error {
					return chromedp.Run(ctx,
						chromedp.Sleep(2*time.Second),
						chromedp.Click(`input[id="username"]`, chromedp.ByQuery),
						chromedp.Clear(`input[id="username"]`, chromedp.ByQuery),
						chromedp.SetValue(`input[id="username"]`, email, chromedp.ByQuery),
						chromedp.ActionFunc(func(ctx context.Context) error {
							log.Println("Filled email")
							return nil
						}),
						chromedp.Click(`button[type="submit"]`, chromedp.ByQuery),
					)
				},
			},
			{
				Checker: func(ctx context.Context) {
					chromedp.WaitReady(`input[name="username"][readonly]`, chromedp.ByQuery).Do(ctx)
				},
				Action: func(ctx context.Context) error {
					log.Println("Username is already filled")
					return nil
				},
			},
		}, 5*time.Second),
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("waiting for login form")
			return nil
		}),
		chromedp.WaitReady(`input[id="password"]`, chromedp.ByQuery),
		chromedp.SendKeys(`input[id="password"]`, password, chromedp.ByQuery),
		chromedp.Click(`button[type="submit"]`, chromedp.ByQuery),
	)
}

func makeBooking(ctx context.Context, coworkingLocationID string, date string) error {
	layout := "Jan 2, 2006"
	// We do not need to check the error as this was already checked
	d, _ := time.Parse(layout, date)

	now := time.Now()

	if d.Sub(now) > 31*24*time.Hour {
		return ErrDateInOlderThanOneMonthFuture
	}

	bearerToken, err := getBearerToken(ctx)

	if err != nil {
		return err
	}

	weworkLocation, err := FetchWeWorkLocation(ctx, bearerToken, coworkingLocationID)

	if err != nil {
		return err
	}

	return makeBookingRequest(ctx, bearerToken, d, weworkLocation)
}

func getPage(ctx context.Context) (string, error) {
	currentPage := ""

	run := func(ctx context.Context) error {
		return chromedp.Run(ctx,
			chromedp.Navigate(`https://members.wework.com/workplaceone/content2/your-bookings`),
			chromedp.ActionFunc(func(ctx context.Context) error {
				ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()

				resultCh := make(chan string, 1)

				go func() {
					chromedp.WaitVisible(`//button[text()="Member log in"]`, chromedp.BySearch).Do(ctx)
					select {
					case resultCh <- PageLogin:
					case <-ctx.Done(): // Ensure no goroutine hangs if the context is canceled
					}
				}()
				go func() {
					chromedp.WaitReady(`wework-ondemand-my-bookings`, chromedp.ByQuery).Do(ctx)
					select {
					case resultCh <- PageReserve:
					case <-ctx.Done(): // Ensure no goroutine hangs if the context is canceled
					}
				}()

				select {
				case result := <-resultCh:
					currentPage = result
				case <-ctx.Done():
					return errors.New("timed out waiting for page to load")
				}

				return nil
			}))
	}

	if err := run(ctx); err != nil {
		log.Println("Error navigating to bookings page:", err)

		// Retry one more time
		return currentPage, run(ctx)
	}

	return currentPage, nil
}

// This holds the checker and the action that should be done when true
type BrowserSwitchAction struct {
	Checker func(ctx context.Context)
	Action  func(ctx context.Context) error
}

func raceItems(ctx context.Context, actions []BrowserSwitchAction, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resultCh := make(chan int, 1)

	for i, action := range actions {
		go func(ctx context.Context) {
			action.Checker(ctx)
			select {
			case resultCh <- i:
			case <-ctx.Done(): // Ensure no goroutine hangs if the context is canceled
			}
		}(ctx)
	}

	select {
	case result := <-resultCh:
		return actions[result].Action(ctx)
	case <-ctx.Done():
		return errors.New("timed out waiting for page to load")
	}
}

func raceItemsChromeFn(ctx context.Context, actions []BrowserSwitchAction, timeout time.Duration) chromedp.ActionFunc {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		return raceItems(ctx, actions, timeout)
	})
}
