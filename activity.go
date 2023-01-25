// main.go
package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/nxadm/tail"
	"github.com/pocketbase/pocketbase"
	//"github.com/pocketbase/pocketbase/forms"
	"github.com/pocketbase/pocketbase/models"
)

func doTail(a *pocketbase.PocketBase) {
	reOpening := regexp.MustCompile(`Opening stream on module (?P<module>[A-Z]) for client (?P<client>[^\s]+)\s+(?P<clientmod>.) with sid \d{1,} by user (?P<user>.*)`)
	reClosing := regexp.MustCompile(`Closing stream of module ([A-Z])`)

	time.Sleep(5 * time.Second)
	t, err := tail.TailFile(
		"/var/log/syslog", tail.Config{Follow: true, ReOpen: true})
	if err != nil {
		panic(err)
	}

	collection, err := a.Dao().FindCollectionByNameOrId("activity")
	if err != nil {
		log.Fatal(err)
	}

	// Print the text of each received line
	tzLocation, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		log.Fatal(err)
	}
	for line := range t.Lines {
		parts := strings.Split(line.Text, " ")
		if len(parts) < 3 || parts[2] != "xlxd:" {
			continue
		}
		if strings.Contains(line.Text, "Sending connect packet to XLX peer") {
			continue
		}
		log.Println(line.Text)
		groups := reOpening.FindStringSubmatch(line.Text)
		if len(groups) == 5 {
			record := models.NewRecord(collection)
			ts, err := time.ParseInLocation(time.RFC3339Nano, parts[0], tzLocation)
			if err != nil {
				log.Fatal(err)
			}
			via := groups[2]
			if groups[3] != " " {
				via = via + "-" + groups[3]
			}
			record.Set("ts", ts.UnixMilli())
			record.Set("system", "299")
			record.Set("module", groups[1])
			record.Set("call", strings.Split(groups[4], " ")[0])
			record.Set("via", via)
			if err := a.Dao().SaveRecord(record); err != nil {
				log.Fatal(err)
			}
		}
		groups = reClosing.FindStringSubmatch(line.Text)
		if len(groups) == 2 {
			record := models.NewRecord(collection)
			ts, err := time.ParseInLocation(time.RFC3339Nano, parts[0], tzLocation)
			if err != nil {
				log.Fatal(err)
			}
			record.Set("ts", ts.UnixMilli())
			record.Set("system", "299")
			record.Set("module", parts[7])
			record.Set("call", "")
			record.Set("via", "")
			if err := a.Dao().SaveRecord(record); err != nil {
				log.Fatal(err)
			}
		}
	}

	fmt.Println("about to cleanup")
	t.Cleanup()
	fmt.Println("cleaned")
}

func main() {
	fmt.Println("Activity monitor")
	fmt.Println(os.Args)
	app := pocketbase.New()

	if err := app.Bootstrap(); err != nil {
		log.Fatal(err)
	}

	go doTail(app)

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}

}
