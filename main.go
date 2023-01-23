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
	"github.com/pocketbase/pocketbase/forms"
	"github.com/pocketbase/pocketbase/models"
)

func doTail(a *pocketbase.PocketBase) {
	reOpening := regexp.MustCompile(`.*Opening stream on module (?P<module>.) for client (?P<client>[^\s]+)\s+(?P<clientmod>.) with sid \d{1,} by user (?P<user>.*)`)

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
		log.Println(line.Text)
		parts := strings.Split(line.Text, " ")
		if len(parts) < 3 {
			log.Println("============================= MISSING PARTS =================")
			continue
		}
		if parts[2] == "xlxd:" && parts[3] == "Opening" {
			groups := reOpening.FindStringSubmatch(line.Text)
			record := models.NewRecord(collection)
			ts, err := time.ParseInLocation(time.RFC3339Nano, parts[0], tzLocation)
			if err != nil {
				log.Fatal(err)
			}
			/*
				record.Set("ts", ts.Format(time.RFC3339))
				record.Set("system", "299")
				record.Set("module", parts[7])
				record.Set("call", groups[4])
				record.Set("via", groups[2]+"-"+groups[3])
				if err := a.Dao().SaveRecord(record); err != nil {
					log.Fatal(err)
				}
			*/
			form := forms.NewRecordUpsert(a, record)

			// or form.LoadRequest(r, "")
			form.LoadData(map[string]any{
				"ts":     ts.UnixMilli(),
				"system": "299",
				"module": parts[7],
				"call":   strings.Split(groups[4], " ")[0],
				"via":    groups[2] + "-" + groups[3],
			})

			// validate and submit (internally it calls app.Dao().SaveRecord(record) in a transaction)
			if err := form.Submit(); err != nil {
				log.Fatal(err)
			}
		} else if parts[2] == "xlxd:" && parts[3] == "Closing" {
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
		} else if strings.HasPrefix(parts[2], "ambed") && parts[3] == "Vocodec" {
			/*
				record := models.NewRecord(collection)
				ts, err := time.ParseInLocation(time.RFC3339Nano, parts[0], tzLocation)
				if err != nil {
					log.Fatal(err)
				}
				record.Set("ts", ts.UnixMilli())
				record.Set("system", "299X")
				record.Set("module", "")
				record.Set("call", parts[5]+parts[6]+parts[7])
				record.Set("via", parts[8])
				if err := a.Dao().SaveRecord(record); err != nil {
					log.Fatal(err)
				}
			*/
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
