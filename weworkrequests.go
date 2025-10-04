package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
	"resty.dev/v3"
)

type WeWorkLocation struct {
	Reservable struct {
		Capacity      int    `json:"capacity"`
		KubeID        string `json:"KubeId"`
		CwmSpaceID    int    `json:"cwmSpaceId"`
		CwmSpaceCount int    `json:"cwmSpaceCount"`
	} `json:"reservable"`
	UUID           string `json:"uuid"`
	InventoryUUID  string `json:"inventoryUuid"`
	ImageURL       string `json:"imageUrl"`
	HeaderImageURL string `json:"headerImageUrl"`
	Capacity       int    `json:"capacity"`
	Credits        int    `json:"credits"`
	Location       struct {
		Description       string `json:"description"`
		SupportEmail      string `json:"supportEmail"`
		PhoneNormalized   string `json:"phoneNormalized"`
		Currency          string `json:"currency"`
		PrimaryTeamMember struct {
			Name          string `json:"name"`
			BusinessTitle string `json:"businessTitle"`
			ImageURL      string `json:"imageUrl"`
		} `json:"primaryTeamMember"`
		Amenities []struct {
			UUID      string `json:"uuid"`
			Name      string `json:"name"`
			Highlight bool   `json:"highlight"`
		} `json:"amenities"`
		Details struct {
			HasExtendedHours bool `json:"hasExtendedHours"`
		} `json:"details"`
		TransitInfo struct {
			Bike    string `json:"bike"`
			Bus     string `json:"bus"`
			Ferry   string `json:"ferry"`
			Freeway string `json:"freeway"`
			Metro   string `json:"metro"`
			Parking string `json:"parking"`
		} `json:"transitInfo"`
		MemberEntranceInstructions string `json:"memberEntranceInstructions"`
		ParkingInstructions        string `json:"parkingInstructions"`
		CommunityBarFloor          struct {
			Name string `json:"name"`
		} `json:"communityBarFloor"`
		TimezoneOffset     string `json:"timezoneOffset"`
		TimeZoneIdentifier string `json:"timeZoneIdentifier"`
		TimeZoneWinID      string `json:"timeZoneWinId"`
		Images             []struct {
			UUID     string `json:"uuid"`
			Caption  string `json:"caption"`
			Category string `json:"category"`
			URL      string `json:"url"`
		} `json:"images"`
		UUID      string  `json:"uuid"`
		Name      string  `json:"name"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Address   struct {
			Line1   string `json:"line1"`
			Line2   string `json:"line2"`
			City    string `json:"city"`
			State   string `json:"state"`
			Country string `json:"country"`
			Zip     string `json:"zip"`
		} `json:"address"`
		TimeZone               string  `json:"timeZone"`
		Distance               float32 `json:"distance"`
		HasThirdPartyDisplay   bool    `json:"hasThirdPartyDisplay"`
		IsMigrated             bool    `json:"isMigrated"`
		SpaceAvailabilityCount int     `json:"spaceAvailabilityCount"`
		Franchise              string  `json:"franchise"`
		AccountType            int     `json:"accountType"`
		AffiliateSpaceType     int     `json:"affiliateSpaceType"`
	} `json:"location"`
	OpenTime           string `json:"openTime"`
	CloseTime          string `json:"closeTime"`
	CancellationPolicy string `json:"cancellationPolicy"`
	OperatingHours     []struct {
		DayOfWeek int    `json:"dayOfWeek"`
		Day       string `json:"day"`
		Open      string `json:"open"`
		Close     string `json:"close"`
		IsClosed  bool   `json:"isClosed"`
	} `json:"operatingHours"`
	ProductPrice struct {
		UUID        string `json:"uuid"`
		ProductUUID string `json:"productUuid"`
		Price       struct {
			Currency string  `json:"currency"`
			Amount   float32 `json:"amount"`
		} `json:"price"`
		RateUnit             int `json:"rateUnit"`
		HalfHourCreditPrices []struct {
			Offset int     `json:"offset"`
			Amount float64 `json:"amount"`
		} `json:"halfHourCreditPrices"`
	} `json:"productPrice"`
	Seat struct {
		Total     int `json:"total"`
		Available int `json:"available"`
	} `json:"seat"`
	SeatsAvailable     int  `json:"seatsAvailable"`
	Order              int  `json:"order"`
	IsHybridSpace      bool `json:"isHybridSpace"`
	AffiliateSpaceType int  `json:"affiliateSpaceType"`
	SpaceTypeID        int  `json:"SpaceTypeID"`
}

type WeWorkLocationsResponse struct {
	Limit               int `json:"limit"`
	Offset              int `json:"offset"`
	GetSharedWorkspaces struct {
		Workspaces []WeWorkLocation `json:"workspaces"`
	} `json:"getSharedWorkspaces"`
}

func FetchWeWorkLocation(ctx context.Context, token string, locationID string) (WeWorkLocation, error) {
	request := resty.New().R().SetContext(ctx).SetAuthToken(token)

	var locationsResponse WeWorkLocationsResponse

	response, err := request.SetResult(&locationsResponse).
		Get(fmt.Sprintf("https://members.wework.com/workplaceone/api/spaces/get-spaces?locationUUIDs=%s", locationID))

	if err != nil {
		return WeWorkLocation{}, err
	}

	if response.IsError() {
		return WeWorkLocation{}, fmt.Errorf("error fetching locations: %s", response.Status())
	}

	if len(locationsResponse.GetSharedWorkspaces.Workspaces) == 0 {
		return WeWorkLocation{}, errors.New("no locations found")
	}

	return locationsResponse.GetSharedWorkspaces.Workspaces[0], nil
}

func getBearerToken(ctx context.Context) (string, error) {
	var token string

	if err := chromedp.Run(ctx,
		chromedp.EvaluateAsDevTools(`(function() {
			const baseItems = localStorage.getItem('Auth0Config');
			
			if (!baseItems) {
				throw new Error("could not find Auth0Config in local storage");
			}
			const config = JSON.parse(baseItems);

			const { clientId, authorizationParams: { scope } } = config;

			const items = localStorage.getItem('@@auth0spajs@@::' + clientId + '::wework::openid ' + scope);

			if (!items) {
				throw new Error("could not find auth0 items in local storage");
			}

			return JSON.parse(items).body.access_token;
		})()`, &token),
	); err != nil {
		return "", err
	}

	if token == "" {
		return "", errors.New("could not find bearer token")
	}

	return token, nil
}

type BookingRequest struct {
	ApplicationType      string   `json:"ApplicationType"`
	PlatformType         string   `json:"PlatformType"`
	SpaceType            int      `json:"SpaceType"`
	ReservationID        string   `json:"ReservationID"`
	TriggerCalendarEvent bool     `json:"TriggerCalendarEvent"`
	MailData             MailData `json:"MailData"`
	LocationType         int      `json:"LocationType"`
	UTCOffset            string   `json:"UTCOffset"`
	CreditRatio          int      `json:"CreditRatio"`
	LocationID           string   `json:"LocationID"`
	SpaceID              string   `json:"SpaceID"`
	WeWorkSpaceID        string   `json:"WeWorkSpaceID"`
	StartTime            string   `json:"StartTime"`
	EndTime              string   `json:"EndTime"`
}

type MailData struct {
	DayFormatted       string `json:"dayFormatted"`
	StartTimeFormatted string `json:"startTimeFormatted"`
	EndTimeFormatted   string `json:"endTimeFormatted"`
	LocationAddress    string `json:"locationAddress"`
	CreditsUsed        string `json:"creditsUsed"`
	Capacity           string `json:"Capacity"`
	TimezoneUsed       string `json:"TimezoneUsed"`
	TimezoneIana       string `json:"TimezoneIana"`
	TimezoneWin        string `json:"TimezoneWin"`
	StartDateTime      string `json:"startDateTime"`
	EndDateTime        string `json:"endDateTime"`
	LocationName       string `json:"locationName"`
	LocationCity       string `json:"locationCity"`
	LocationCountry    string `json:"locationCountry"`
	LocationState      string `json:"locationState"`
}

type BookingResponse struct {
	BookingStatus string   `json:"BookingStatus"`
	Errors        []string `json:"Errors"`
	ReservationID string   `json:"ReservationID"`
	WeworkUUID    string   `json:"WeWorkUUID"`
}

func makeBookingRequest(ctx context.Context, token string, date time.Time, space WeWorkLocation) error {
	request := resty.New().R()

	request.SetAuthToken(token)

	request.SetContext(ctx)

	requestData := BookingRequest{
		ApplicationType:      "WorkplaceOne",
		PlatformType:         "WEB",
		SpaceType:            4,
		ReservationID:        "",
		TriggerCalendarEvent: false,
		MailData: MailData{
			DayFormatted:       GetEmailDateFormated(date),
			StartTimeFormatted: space.OpenTime,
			EndTimeFormatted:   space.CloseTime,
			LocationAddress:    space.Location.Address.Line1,
			CreditsUsed:        "2",
			Capacity:           "1",
			TimezoneUsed:       "GMT +02:00",
			TimezoneIana:       space.Location.TimeZoneIdentifier,
			TimezoneWin:        space.Location.TimeZoneWinID,
			StartDateTime:      fmt.Sprintf("%s 06:00", date.Format(time.DateOnly)),
			EndDateTime:        fmt.Sprintf("%s 23:59", date.Format(time.DateOnly)),
			LocationName:       space.Location.Name,
			LocationCity:       space.Location.Address.City,
			LocationCountry:    space.Location.Address.Country,
			LocationState:      space.Location.Address.State,
		},
		LocationType:  2,
		UTCOffset:     space.Location.TimezoneOffset,
		CreditRatio:   20,
		LocationID:    space.Location.UUID,
		SpaceID:       space.Reservable.KubeID,
		WeWorkSpaceID: space.UUID,
		StartTime:     fmt.Sprintf("%sT04:00:00Z", date.Format(time.DateOnly)),
		EndTime:       fmt.Sprintf("%sT21:59:00Z", date.Format(time.DateOnly)),
	}

	request.SetBody(requestData)

	// var bookingResponse BookingResponse

	// response, err := request.SetResult(&bookingResponse).
	// 	Post("https://members.wework.com/workplaceone/api/common-booking/")

	// if err != nil {
	// 	return err
	// }

	// if response.IsError() {
	// 	return fmt.Errorf("error making booking request: %s", response.Status())
	// }

	// if bookingResponse.BookingStatus != "BookingSuccess" {
	// 	return fmt.Errorf("booking not confirmed: %v", bookingResponse.Errors)
	// }

	return nil
}
