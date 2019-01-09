// Copyright © 2018 Zadara Storage
// Originally Authored By: Jeremy Brown <jeremy@zadarastorage.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/influxdata/influxdb/client/v2"
	jww "github.com/spf13/jwalterweatherman"
)

func (d *dataio) createDevTypes() (err error) {
	var name string

	err = d.input.QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'dev_types'").Scan(&name)

	switch {
	case err == sql.ErrNoRows:
		jww.DEBUG.Printf("table 'dev_types' needs to be created in metering db: %v", d.inputDB)

		if _, err := d.input.Exec("CREATE TABLE dev_types (dev_type integer, dev_name varchar)"); err != nil {
			return err
		}

		q := "INSERT INTO dev_types (dev_type, dev_name) VALUES (?, ?)"

		tx, err := d.input.Beginx()

		if err != nil {
			return err
		}

		for t, n := range devTypes {
			if _, err := tx.Exec(q, t, n); err != nil {
				tx.Rollback()
				return err
			}
		}

		tx.Commit()
	case err != nil:
		return err
	default:
		jww.DEBUG.Printf("table 'dev_types' already exists in metering db: %v", d.inputDB)
	}

	return nil
}

func (d *dataio) findMeteringTables() (tables []string, err error) {
	// hard code specific tables we're interested in
	t := []string{"metering_info", "metering_sys_info", "metering_zcache_info"}

	if err != nil {
		return nil, err
	}

	for _, name := range t {
		var count int

		// check there is actual data to process
		if err = d.input.QueryRow("SELECT count(*) FROM " + name).Scan(&count); err != nil {
			return nil, err
		}

		jww.DEBUG.Printf("count for table %v: %v", name, count)

		if count > 0 {
			jww.DEBUG.Printf("metering info in db %v found in table: %v", d.inputDB, name)
			tables = append(tables, name)
		}
	}

	return tables, nil
}

func (d *dataio) ingestMeteringTable(tableName string, vpsa string) (err error) {
	count := 0
	commitSize := 10000
	var bp client.BatchPoints
	var interval int64
	var matchCount int
	var measurement string
	var queryName string
	var remainingChunks int
	var tableCount int

	bp, err = client.NewBatchPoints(client.BatchPointsConfig{
		Database:  "zadara",
		Precision: "s",
	})

	if err != nil {
		return err
	}

	if err = d.input.QueryRow("SELECT count(*) FROM " + tableName).Scan(&tableCount); err != nil {
		return err
	}

	remainingChunks = tableCount / commitSize

	// Normalize us to ms for consistency
	if tableName == "metering_info" {
		if err = d.input.QueryRow("SELECT count(*) FROM sqlite_master where name = 'metering_info' and sql like('%total_resp_tm_ms%')").Scan(&matchCount); err != nil {
			return err
		}

		if matchCount == 0 {
			queryName = "metering_info_us"
		} else {
			queryName = "metering_info_ms"
		}
	} else {
		queryName = tableName
	}

	q := sqlSelect[queryName]
	jww.DEBUG.Printf("executing metering query: %v", q)
	rows, err := d.input.Queryx(q)

	if err != nil {
		return err
	}

	defer rows.Close()

	jww.INFO.Printf("ingesting table %v in database %v with record count: %v", tableName, d.inputDB, tableCount)

	for rows.Next() {
		count++
		var timestamp time.Time
		fields := make(map[string]interface{})
		row := make(map[string]interface{})
		tags := make(map[string]string)

		tags["vpsa"] = vpsa

		if err = rows.MapScan(row); err != nil {
			return err
		}

		// any non-"special" value that is []uint8 (string) type is a tag, all others are fields
		for n, v := range row {
			if v != nil {
				switch reflect.TypeOf(v).String() {
				case "[]uint8":
					if n == "measurement" {
						measurement = string(v.([]uint8))
					} else {
						tags[n] = string(v.([]uint8))
					}
				default:
					// For "rstjobs" types - apparently the dev_server_name and dev_target_name can be 0 - this causes
					// below logic to assume it should be a field rather than a tag - which creates duplicate column
					// names like "dev_server_name" and "dev_server_name_1" - one being a field, the other being a tag.
					// Detect these special names and force them to be a tag.
					if n == "dev_server_name" || n == "dev_target_name" {
						tags[n] = fmt.Sprintf("%v", v)
					} else if n == "time" {
						timestamp = v.(time.Time)
					} else if n == "interval" {
						interval = v.(int64)
					} else {
						fields[n] = v
					}
				}
			}
		}

		if len(fields) > 0 {
			pt, err := client.NewPoint(
				measurement,
				tags,
				fields,
				timestamp,
			)

			if err != nil {
				return err
			}

			bp.AddPoint(pt)
		}

		// writing out points in smaller batches is more memory efficient for both this program and influx
		if (count % commitSize) == 0 {
			remainingChunks--

			if err = d.output.Write(bp); err != nil {
				return err
			}

			// need to create a new batch point variable, as apparently writing a batch does not drain the previous
			bp, err = client.NewBatchPoints(client.BatchPointsConfig{
				Database:  "zadara",
				Precision: "s",
			})

			if err != nil {
				return err
			}

			jww.DEBUG.Printf("table %v in database %v reached record %v, remaining chunks before final: %v", tableName, d.inputDB, count, remainingChunks)
		}

	}

	jww.INFO.Printf("table %v in database %v reached final record %v, flushing final chunk", tableName, d.inputDB, count)

	if err = d.output.Write(bp); err != nil {
		return err
	}

	jww.DEBUG.Printf("table %v for VPSA %v has interval: %v", tableName, vpsa, interval)

	return nil
}

func processMeteringFiles(extractPath string, vpsa string) (err error) {
	influxHTTP := "http://127.0.0.1:8086"

	meteringFiles, err := findMeteringFiles(extractPath)

	if err != nil {
		return err
	}

	jww.INFO.Printf("metering databases to analyze: %v", meteringFiles)

	filec, errc := make(chan string), make(chan error)

	for _, meteringFile := range meteringFiles {
		go func(meteringFile string) {
			d, err := newDataIO(meteringFile, influxHTTP)

			if err != nil {
				errc <- err
				return
			}

			defer d.input.Close()
			defer d.output.Close()

			if err = d.createDevTypes(); err != nil {
				errc <- err
				return
			}

			tables, err := d.findMeteringTables()

			if err != nil {
				errc <- err
				return
			}

			jww.DEBUG.Printf("found metering tables in db %v: %v", d.inputDB, tables)

			tablec, terrc := make(chan string), make(chan error)

			for _, table := range tables {
				go func(table string) {
					jww.DEBUG.Printf("processing metering table: %v", table)

					if err = d.ingestMeteringTable(table, vpsa); err != nil {
						jww.DEBUG.Printf("hit error in db %v while querying table: %v", d.inputDB, table)
						terrc <- err
						return
					}

					tablec <- table
				}(table)
			}

			for i := 0; i < len(tables); i++ {
				select {
				case table := <-tablec:
					jww.INFO.Printf("processed metering table %v inside db: %v", table, d.inputDB)
				case terr := <-terrc:
					errc <- terr
				}
			}

			filec <- meteringFile
		}(meteringFile)
	}

	for i := 0; i < len(meteringFiles); i++ {
		select {
		case meteringFile := <-filec:
			jww.INFO.Printf("processed metering database: %v", meteringFile)
		case err := <-errc:
			return err
		}
	}

	return nil
}

func findMeteringFiles(extractPath string) (meteringFiles []string, err error) {
	// assume all files in metering root with .db extension are metering data and process
	err = filepath.Walk(extractPath, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			jww.ERROR.Printf("could not walk path: %v\n", extractPath)
			return err
		}

		if filepath.Ext(path) == ".db" {
			jww.DEBUG.Printf("found metering file: %v", path)
			meteringFiles = append(meteringFiles, path)
		}

		return nil
	})

	if err != nil {
		jww.ERROR.Printf("error walking the path: %v", extractPath)
		return nil, err
	}

	if len(meteringFiles) == 0 {
		return nil, fmt.Errorf("could not locate any metering files in location: %v", extractPath)
	}

	return meteringFiles, nil
}

func getVPSANameFromPath(meteringFilePath string) (vpsa string, err error) {
	parts := strings.Split(meteringFilePath, string(os.PathSeparator))

	if len(parts) < 1 {
		return "", fmt.Errorf("could not split path by directory separator: %v", meteringFilePath)
	}

	return parts[len(parts)-2], nil
}

func isDirEmpty(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer f.Close()

	// read in ONLY one file
	_, err = f.Readdir(1)

	// and if the file is EOF... well, the dir is empty.
	if err == io.EOF {
		return true, nil
	}
	return false, err
}
