// main.go
// vim:noet:ts=4
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
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/daos"
	"github.com/pocketbase/pocketbase/models"
)

func getLastTime(dao *daos.Dao) (int64, error) {
	collection, err := dao.FindCollectionByNameOrId("activity")
	if err != nil {
		return 0, err
	}

	query := dao.RecordQuery(collection).
		AndWhere(dbx.HashExp{"system": "299"}).
		OrderBy("ts DESC").
		Limit(1)

	rows := []dbx.NullStringMap{}
	if err := query.All(&rows); err != nil {
		return 0, err
	}

	return int64(models.NewRecordsFromNullStringMaps(collection, rows)[0].GetFloat("ts")), nil
}

func doTail(a *pocketbase.PocketBase) {
	onair := make(map[string]string)	// map of module to last record id
	reOpening := regexp.MustCompile(`Opening stream on module (?P<module>[A-Z]) for client (?P<client>[^\s]+)\s+(?P<clientmod>.) with sid \d{1,} by user (?P<user>.*)`)
	reClosing := regexp.MustCompile(`Closing stream of module ([A-Z])`)

	time.Sleep(1 * time.Second)
	t, err := tail.TailFile(
		"/var/log/syslog", tail.Config{Follow: true, ReOpen: true})
	if err != nil {
		panic(err)
	}

	collection, err := a.Dao().FindCollectionByNameOrId("activity")
	if err != nil {
		log.Fatal(err)
	}

	lastTime, err := getLastTime(a.Dao())
	log.Printf("lastTime %d\n", lastTime)

	time.Sleep(4 * time.Second)
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
		ts, err := time.ParseInLocation(time.RFC3339Nano, parts[0], tzLocation)
		if err != nil {
			// tail sometimes leaves a truncated date
			// log.Fatal(err)
			ts = time.Now()		// or maybe last parsed time plus inc
			log.Printf("Error: Unable to parse time %s\n", parts[0])
		}
		uTs := ts.UnixMilli()
		if uTs <= lastTime {
			continue
		}
		log.Println(line.Text)
		groups := reOpening.FindStringSubmatch(line.Text)
		if len(groups) == 5 {
			record := models.NewRecord(collection)
			via := groups[2]
			if groups[3] != " " {
				via = via + "-" + groups[3]
			}
			record.Set("ts", uTs)
			record.Set("tsoff", 0)
			record.Set("system", "299")
			record.Set("module", groups[1])
			record.Set("call", strings.Split(groups[4], " ")[0])
			record.Set("via", via)
			if err := a.Dao().SaveRecord(record); err != nil {
				log.Fatal(err)
			}
			// save the Id of the onair record
			onair[groups[1]] = record.Id;
			log.Printf("+++ on +++ %s on %s %d %s", strings.Split(groups[4], " ")[0], groups[1], uTs, record.Id);
		}
		groups = reClosing.FindStringSubmatch(line.Text)
		if len(groups) == 2 {
			module := parts[7]
			id, ok := onair[module]
			log.Printf("--- off --   mod %s %s %d", module, id, uTs);
			if ok {
				record, err := a.Dao().FindRecordById("activity", id)
				if err != nil {
					log.Fatal(err)
				}
				record.Set("tsoff", uTs)
				if err := a.Dao().SaveRecord(record); err != nil {
					log.Fatal(err)
				}
			} else {
			  log.Printf("!!! off with no on !!! mod %s", module);
			}
		}
	}

	fmt.Println("about to cleanup tailing")
	t.Cleanup()
	fmt.Println("clean")
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
