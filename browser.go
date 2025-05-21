package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/chromedp/chromedp"
)

func withRetries(originalOpCtx context.Context, attempts int, sleep time.Duration, operationName string, fn func(attemptCtx context.Context) error) error {
    var lastErr error
    for i := 0; i < attempts; i++ {
        if i > 0 {
            // Check if the original context has been cancelled before sleeping or retrying
            if originalOpCtx.Err() != nil {
                log.Printf("Context cancelled before retrying operation '%s' (attempt %d/%d). Last error: %v", operationName, i+1, attempts, lastErr)
                // If lastErr is nil, it means context was cancelled before first retry, return context error.
                // Otherwise, prefer returning the actual last operational error.
                if lastErr == nil {
                    return originalOpCtx.Err()
                }
                return lastErr
            }
            log.Printf("Retrying operation '%s' (attempt %d/%d) after error: %v", operationName, i+1, attempts, lastErr)
            time.Sleep(sleep)
        }
        
        // Check if the original context has been cancelled just before the current attempt
        if originalOpCtx.Err() != nil {
            log.Printf("Context cancelled before attempting operation '%s' (attempt %d/%d). Last error (if any): %v", operationName, i+1, attempts, lastErr)
            if lastErr == nil {
                 return originalOpCtx.Err()
            }
            return lastErr
        }

        lastErr = fn(originalOpCtx) // Pass the original context to each attempt of fn
        
        if lastErr == nil {
            return nil // Success
        }
    }
    log.Printf("Operation '%s' failed after %d attempts, last error: %v", operationName, attempts, lastErr)
    return lastErr // All attempts failed
}

const PageLogin = "login"
const PageReserve = "reserve"

func login(ctx context.Context, email string, password string) error {
	return chromedp.Run(ctx,
		chromedp.Click(`//button[text()="Member log in"]`, chromedp.BySearch),
		chromedp.WaitReady(`input[type="email"][id="1-email"]`, chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("waiting for login form")
			return nil
		}),
		chromedp.Click(`input[type="email"][id="1-email"]`, chromedp.ByQuery),
		chromedp.SendKeys(`input[type="email"][id="1-email"]`, email, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Filled email")
			return nil
		}),
		chromedp.SendKeys(`input[type="password"][id="1-password"]`, password, chromedp.ByQuery),
		chromedp.Click(`button[type="submit"][id="1-submit"]`, chromedp.ByQuery),
	)
}

func makeBooking(ctx context.Context, coworkingName string, date string) error {
	if err := chromedp.Run(ctx,
		chromedp.SetValue(`yardi-control-date input`, date, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Filled date and waited")
			return nil
		}),
		chromedp.WaitVisible(`#main-content .loading-block`, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Loading block visible")
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
			if err := chromedp.Click(`//div[contains(@class, "modal-footer")]//button[text()="OK"]`, chromedp.BySearch).Do(wCtx); err != nil {
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
    var liID string

    operationToRetry := func(attemptCtx context.Context) error { // attemptCtx is the ctx from withRetries (i.e., getCoworkingIDFromName's original ctx)
        // Create a new 10-second timeout context for this specific chromedp.Run attempt, derived from attemptCtx
        opCtx, cancel := context.WithTimeout(attemptCtx, 10*time.Second)
        defer cancel()

        err := chromedp.Run(opCtx, // Use the 10s opCtx here for this attempt
            chromedp.AttributeValue(fmt.Sprintf(`//div[text()='%s']/ancestor::li`, coworkingName), "id", &liID, nil),
        )
        return err
    }

    err := withRetries(ctx, 3, 2*time.Second, fmt.Sprintf("getCoworkingIDFromName for '%s'", coworkingName), operationToRetry)
    return liID, err
}

func getPage(ctx context.Context) (string, error) {
    var currentPage string

    operationToRetry := func(attemptCtx context.Context) error { // attemptCtx is the ctx from withRetries (i.e., getPage's original ctx)
        errRun := chromedp.Run(attemptCtx, // This context is used for the overall chromedp.Run
            chromedp.Navigate(`https://members.wework.com/workplaceone/content2/bookings/desks`),
            chromedp.ActionFunc(func(runCtx context.Context) error { // runCtx is from chromedp.Run, derived from attemptCtx
                // This is the 10-second timeout for the actual page interaction for this attempt
                pageLoadCtx, cancel := context.WithTimeout(runCtx, 10*time.Second)
                defer cancel()

                resultCh := make(chan string, 1)

                go func() {
                    if err := chromedp.WaitVisible(`//button[text()="Member log in"]`, chromedp.BySearch).Do(pageLoadCtx); err == nil {
                        select {
                        case resultCh <- PageLogin:
                        case <-pageLoadCtx.Done():
                        }
                    } else if pageLoadCtx.Err() == nil { // Error from WaitVisible, but pageLoadCtx itself not timed out
                        // log.Printf("Debug: Login element not found or other error: %v", err)
                    }
                }()

                go func() {
                    if err := chromedp.WaitReady(`wework-member-web-city-selector`, chromedp.ByQuery).Do(pageLoadCtx); err == nil {
                        select {
                        case resultCh <- PageReserve:
                        case <-pageLoadCtx.Done():
                        }
                    } else if pageLoadCtx.Err() == nil { // Error from WaitReady, but pageLoadCtx itself not timed out
                        // log.Printf("Debug: Reserve element not found or other error: %v", err)
                    }
                }()

                select {
                case res := <-resultCh:
                    currentPage = res
                    return nil
                case <-pageLoadCtx.Done():
                    return errors.New("timed out waiting for page to load") // This error will be retried by withRetries
                }
            }),
        )
        return errRun
    }
    
    err := withRetries(ctx, 3, 2*time.Second, "getPageLoad", operationToRetry)
    return currentPage, err
}
