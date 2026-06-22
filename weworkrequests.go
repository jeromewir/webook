package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
	"resty.dev/v3"
)

var ErrWeWorkLocationNotFound = errors.New("wework location not found")

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

type WeWorkProperty struct {
	ID                     string  `json:"id"`
	Title                  string  `json:"title"`
	Address                string  `json:"address"`
	City                   string  `json:"city"`
	Country                string  `json:"country"`
	CoworkingOperatorName  string  `json:"coworkingOperatorName"`
	CoworkingPropertyID    int     `json:"coworkingPropertyId"`
	PropertyTimezoneIANA   string  `json:"propertyTimezoneIana"`
	PropertyTimezoneWin    string  `json:"propertyTimezoneWin"`
	PropertyTimezoneOffset string  `json:"propertyTimezoneOffset"`
	Latitude               float64 `json:"-"`
	Longitude              float64 `json:"-"`
	Position               struct {
		Latitude  float64 `json:"lat"`
		Longitude float64 `json:"lng"`
	} `json:"position"`
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

func FetchWeWorkLocationByName(ctx context.Context, token string, locationName string) (WeWorkLocation, error) {
	properties, err := fetchWeWorkProperties(ctx, token)
	if err != nil {
		return WeWorkLocation{}, err
	}

	property, err := findWeWorkPropertyByName(properties, locationName)
	if err != nil {
		log.Printf("WeWork property lookup for %q returned %d properties: %s", locationName, len(properties), strings.Join(weWorkPropertyNames(properties), ", "))
		return WeWorkLocation{}, err
	}

	log.Printf("WeWork property lookup for %q matched %q (%s)", locationName, property.Title, property.ID)
	return FetchWeWorkLocation(ctx, token, property.ID)
}

func fetchWeWorkProperties(ctx context.Context, token string) ([]WeWorkProperty, error) {
	request := resty.New().R().SetContext(ctx).SetAuthToken(token)

	var properties []WeWorkProperty

	response, err := request.SetResult(&properties).Get("https://members.wework.com/workplaceone/api/Workspace/get-property-list-google-map?offloadToServer=true&isPropSvcCl=false")
	if err != nil {
		return nil, err
	}

	if response.IsError() {
		return nil, fmt.Errorf("error fetching properties: %s", response.Status())
	}

	return properties, nil
}

func findWeWorkPropertyByName(properties []WeWorkProperty, locationName string) (WeWorkProperty, error) {
	wanted := normalizeLocationName(locationName)
	if wanted == "" {
		return WeWorkProperty{}, errors.New("missing wework name")
	}

	for _, property := range properties {
		if normalizeLocationName(property.Title) == wanted {
			return property, nil
		}
	}

	var matches []WeWorkProperty
	for _, property := range properties {
		if strings.Contains(normalizeLocationName(property.Title), wanted) {
			matches = append(matches, property)
		}
	}

	if len(matches) == 1 {
		return matches[0], nil
	}

	if len(matches) > 1 {
		log.Printf("WeWork lookup for %q matched multiple properties: %s", locationName, strings.Join(weWorkPropertyNames(matches), ", "))
		return WeWorkProperty{}, fmt.Errorf("wework name %q matched multiple locations; use the exact name", locationName)
	}

	for _, property := range properties {
		if strings.Contains(normalizeLocationName(property.Address), wanted) {
			matches = append(matches, property)
		}
	}

	if len(matches) == 1 {
		return matches[0], nil
	}

	if len(matches) > 1 {
		log.Printf("WeWork lookup for %q matched multiple property addresses: %s", locationName, strings.Join(weWorkPropertyNames(matches), ", "))
		return WeWorkProperty{}, fmt.Errorf("wework name %q matched multiple locations; use the exact name", locationName)
	}

	return WeWorkProperty{}, fmt.Errorf("%w for name %q", ErrWeWorkLocationNotFound, locationName)
}

func normalizeLocationName(locationName string) string {
	locationName = strings.NewReplacer("œ", "oe", "Œ", "Oe").Replace(locationName)
	decomposed := norm.NFD.String(locationName)
	folded := strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}

		return r
	}, decomposed)

	return strings.ToLower(strings.Join(strings.Fields(folded), " "))
}

func weWorkLocationNames(locations []WeWorkLocation) []string {
	names := make([]string, 0, len(locations))
	for _, location := range locations {
		if location.Location.Name != "" {
			names = append(names, location.Location.Name)
		}
	}

	if len(names) > 10 {
		return append(names[:10], fmt.Sprintf("...and %d more", len(names)-10))
	}

	return names
}

func weWorkPropertyNames(properties []WeWorkProperty) []string {
	names := make([]string, 0, len(properties))
	for _, property := range properties {
		if property.Title != "" {
			names = append(names, property.Title)
		}
	}

	if len(names) > 10 {
		return append(names[:10], fmt.Sprintf("...and %d more", len(names)-10))
	}

	return names
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

func FetchNextBookings(ctx context.Context, token string, startDate string, endDate string) (json.RawMessage, error) {
	values := url.Values{}
	values.Set("isPastBooking", "false")
	values.Set("platFormType", "WEB")
	values.Set("startDate", startDate)
	values.Set("endDate", endDate)

	requestURL := "https://members.wework.com/workplaceone/api/common-booking/get-app-upcoming-bookings?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("error fetching bookings: %s", resp.Status)
	}

	if !json.Valid(body) {
		return nil, errors.New("bookings response was not valid JSON")
	}

	return json.RawMessage(body), nil
}

var makeBookingRequestFunc = makeBookingRequest

// parseTimezoneOffset parses a timezone offset string like "GMT +02:00" or "GMT -05:00"
// and returns the offset in hours as a float64
func parseTimezoneOffset(tzOffset string) (float64, error) {
	// Match patterns like "GMT +02:00", "GMT +2:00", "GMT -05:00", etc.
	re := regexp.MustCompile(`([+-])(\d{1,2}):(\d{2})`)
	matches := re.FindStringSubmatch(tzOffset)

	if len(matches) != 4 {
		return 0, fmt.Errorf("invalid timezone offset format: %s", tzOffset)
	}

	sign := matches[1]
	hours, err := strconv.Atoi(matches[2])
	if err != nil {
		return 0, fmt.Errorf("invalid hours in timezone offset: %s", tzOffset)
	}

	minutes, err := strconv.Atoi(matches[3])
	if err != nil {
		return 0, fmt.Errorf("invalid minutes in timezone offset: %s", tzOffset)
	}

	offset := float64(hours) + float64(minutes)/60.0
	if sign == "-" {
		offset = -offset
	}

	return offset, nil
}

// calculateUTCTime converts a local time to UTC based on the timezone offset
// localTime should be in "HH:MM" format, tzOffset like "GMT +02:00"
func calculateUTCTime(date time.Time, localTime string, tzOffset string) (string, error) {
	offset, err := parseTimezoneOffset(tzOffset)
	if err != nil {
		return "", err
	}

	// Parse local time (e.g., "06:00" or "23:59")
	var hour, minute int
	_, err = fmt.Sscanf(localTime, "%d:%d", &hour, &minute)
	if err != nil {
		return "", fmt.Errorf("invalid time format: %s", localTime)
	}

	// Create a time at the given local time in the location's timezone
	localDateTime := time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, time.UTC)

	// Subtract the offset to get UTC time
	// If offset is +2, local time is 2 hours ahead of UTC, so UTC = local - 2
	utcTime := localDateTime.Add(time.Duration(-offset * float64(time.Hour)))

	return utcTime.Format("2006-01-02T15:04:05Z"), nil
}

func makeBookingRequest(ctx context.Context, token string, date time.Time, space WeWorkLocation) error {
	request := resty.New().R()

	request.SetAuthToken(token)

	request.SetContext(ctx)

	// Calculate UTC times based on local times and timezone offset
	// Local start time is 06:00, end time is 23:59
	startTimeUTC, err := calculateUTCTime(date, "06:00", space.Location.TimezoneOffset)
	if err != nil {
		return fmt.Errorf("failed to calculate start time: %w", err)
	}

	endTimeUTC, err := calculateUTCTime(date, "23:59", space.Location.TimezoneOffset)
	if err != nil {
		return fmt.Errorf("failed to calculate end time: %w", err)
	}

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
			TimezoneUsed:       space.Location.TimezoneOffset,
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
		StartTime:     startTimeUTC,
		EndTime:       endTimeUTC,
	}

	request.SetBody(requestData)

	var bookingResponse BookingResponse

	response, err := request.SetResult(&bookingResponse).
		Post("https://members.wework.com/workplaceone/api/common-booking/")

	if err != nil {
		return err
	}

	if response.IsError() {
		return fmt.Errorf("error making booking request: %s", response.Status())
	}

	if bookingResponse.BookingStatus != "BookingSuccess" {
		return fmt.Errorf("booking not confirmed: %v", bookingResponse.Errors)
	}

	return nil
}
