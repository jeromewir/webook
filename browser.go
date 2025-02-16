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

	if err := chromedp.Run(ctx,
		chromedp.Navigate(`https://members.wework.com/workplaceone/content2/bookings/desks`),
		chromedp.ActionFunc(func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
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
		}),
	); err != nil {
		return "", err
	}

	return currentPage, nil
}
