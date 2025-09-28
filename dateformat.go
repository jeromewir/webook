package main

import (
	"fmt"
	"time"
)

func getOrdinalSuffix(day int) string {
	if day%10 == 1 && day%100 != 11 {
		return "st"
	} else if day%10 == 2 && day%100 != 12 {
		return "nd"
	} else if day%10 == 3 && day%100 != 13 {
		return "rd"
	}
	return "th"
}

func GetEmailDateFormated(date time.Time) string {
	result := fmt.Sprintf("%s, %s %d%s",
		date.Format("Monday"),        // weekday
		date.Format("January"),       // month
		date.Day(),                   // day number
		getOrdinalSuffix(date.Day()), // ordinal
	)

	return result

}
