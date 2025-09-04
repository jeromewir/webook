package main

import (
	"context"
	"errors"
	"fmt"
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
				Action: func(ctx context.Context) {
					chromedp.Run(ctx,
						chromedp.Click(`input[id="username"]`, chromedp.ByQuery),
						chromedp.SetValue(`input[id="username"]`, "", chromedp.ByQuery),
						chromedp.SendKeys(`input[id="username"]`, email, chromedp.ByQuery),
						chromedp.Sleep(500*time.Millisecond),
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
				Action: func(ctx context.Context) {
					log.Println("Username is already filled")
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

func makeBooking(ctx context.Context, coworkingName string, date string) error {
	layout := "Jan 2, 2006"
	// We do not need to check the error as this was already checked
	d, _ := time.Parse(layout, date)

	now := time.Now()

	if err := chromedp.Run(ctx,
		chromedp.WaitVisible(`wework-booking-desk-memberweb .row .loading-block`),
		chromedp.WaitNotPresent(`wework-booking-desk-memberweb .row .loading-block`),
		chromedp.Click(`button[type="button"][class="btn-calendar"]`),
		chromedp.ActionFunc(func(ctx context.Context) error {
			monthBtn := fmt.Sprintf(`dl-date-time-picker div[aria-label="%s"]`, d.Format("Jan 2006"))

			// Check if the date is in more than one month, otherwise return an error
			if d.Sub(now) > 31*24*time.Hour {
				return ErrDateInOlderThanOneMonthFuture
			}

			if d.Year() != now.Year() {
				yearBtn := fmt.Sprintf(`button[type="button"][title="Go to %d"]`, now.Year())

				chromedp.Run(ctx,
					chromedp.Click(`button[type="button"][title="Go to month view"]`),
					chromedp.WaitVisible(yearBtn),
					chromedp.Click(yearBtn),
					chromedp.Click(fmt.Sprintf("//dl-date-time-picker//div[text()='%d']", d.Year())),
				)

				log.Println("Clicked on year", d.Year())
			}

			if d.Month() != now.Month() && d.Year() == now.Year() {
				chromedp.Run(ctx,
					chromedp.Click(`button[type="button"][title="Go to month view"]`),
				)
				log.Println("Clicked on month view button")
			}

			if d.Month() != now.Month() || d.Year() != now.Year() {
				chromedp.Run(ctx,
					chromedp.WaitReady(monthBtn),
					chromedp.Click(monthBtn),
				)
				log.Println("Clicked on month")
			}

			if d.Day() != now.Day() {
				dayBtn := fmt.Sprintf("dl-date-time-picker div[aria-label='%s %d, %d']", d.Format("Jan"), d.Day(), d.Year())

				chromedp.Run(ctx,
					chromedp.Click(dayBtn),
				)
			}

			return nil
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Filled date and waited")
			return nil
		}),
	); err != nil {
		return err
	}

	coworkingID, err := getCoworkingIDFromName(ctx, coworkingName)

	if err != nil {
		return err
	}

	log.Printf("Retrieved coworking ID from name (%s): %s\n", coworkingName, coworkingID)

	baseLi := fmt.Sprintf(`#main-content li[id="%s"]`, coworkingID)

	return chromedp.Run(ctx,
		chromedp.WaitVisible(baseLi, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Booking visible")
			return nil
		}),
		chromedp.Evaluate(fmt.Sprintf(`document.querySelector('li[id="%s"] span[role="button"]').click()`, coworkingID), nil),
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Clicked")
			return nil
		}),
		chromedp.WaitVisible(`memberweb-booking-review-modal .btn-primary .cost`, chromedp.ByQuery),
		chromedp.Click(`memberweb-booking-review-modal .btn-primary`, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// We're waiting for a potential modal to appear, when we don't have enough credits
			wCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			if err := chromedp.Click(`//div[contains(@class, "modal-footer")]//button[text()="Book"]`, chromedp.BySearch).Do(wCtx); err != nil {
				log.Println("No modal appeared", err)
				return nil
			}
			log.Println("Clicked OK")

			return nil
		}),
		chromedp.Click(`//button[text()="Done"]`, chromedp.BySearch),
	)
}

func getCoworkingIDFromName(ctx context.Context, coworkingName string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	liID := ""

	if err := chromedp.Run(ctx,
		chromedp.AttributeValue(fmt.Sprintf(`//div[text()='%s']/ancestor::li`, coworkingName), "id", &liID, nil),
	); err != nil {
		return "", err
	}

	return liID, nil
}

func getPage(ctx context.Context) (string, error) {
	currentPage := ""

	run := func(ctx context.Context) error {
		return chromedp.Run(ctx,
			chromedp.Navigate(`https://members.wework.com/workplaceone/content2/bookings/desks`),
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
					chromedp.WaitReady(`wework-member-web-city-selector`, chromedp.ByQuery).Do(ctx)
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
	Action  func(ctx context.Context)
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
		actions[result].Action(ctx)
	case <-ctx.Done():
		return errors.New("timed out waiting for page to load")
	}

	return nil
}

func raceItemsChromeFn(ctx context.Context, actions []BrowserSwitchAction, timeout time.Duration) chromedp.ActionFunc {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		return raceItems(ctx, actions, timeout)
	})
}
