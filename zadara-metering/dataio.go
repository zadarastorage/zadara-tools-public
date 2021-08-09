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
	"os"

	"github.com/influxdata/influxdb/client/v2"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	jww "github.com/spf13/jwalterweatherman"
)

var (
	devTypes = map[int]string{
		1:  "VOLUME",
		2:  "RAID-GROUP",
		3:  "DRIVE",
		4:  "POOL",
		5:  "SYSTEM",
		6:  "MIRROR",
		7:  "ZCACHE",
		8:  "DMBTRFS",
		9:  "BTRFS",
		10: "BLOCK",
		11: "NOVA",
		12: "SWIFT",
		13: "MIGRATION",
		14: "OBJECT-STORAGE",
	}
	sqlSelect = map[string]string{
		"metering_info_ms": `
SELECT
    "io" as measurement,
    devices.dev_ext_name,
    devices.dev_server_name,
    devices.dev_target_name,
    io_buckets.bucket_name,
    dev_types.dev_name,
    metering_info.interval,
    ROUND(CAST(metering_info.num_ios AS real) / metering_info.interval, 3) AS iops,
    ROUND(CAST(metering_info.bytes AS real) / metering_info.interval, 3) AS bps,
    ROUND(CAST(metering_info.total_resp_tm_ms AS real) / metering_info.num_ios, 3) AS latency_ms,
    metering_info.active_ios,
    metering_info.io_errors,
    metering_info.max_cmd,
    metering_info.max_resp_tm_ms AS max_latency_ms,
    metering_info.time
FROM
    metering_info
    INNER JOIN
        devices
        ON (metering_info.dev_dbid = devices.dev_dbid)
    INNER JOIN
        io_buckets
        ON (devices.dev_type = io_buckets.dev_type)
        AND (metering_info.bucket = io_buckets.bucket)
    INNER JOIN
        dev_types
        ON (devices.dev_type = dev_types.dev_type)
        AND (dev_types.dev_type = io_buckets.dev_type)
ORDER BY
    metering_info.time
`,
		"metering_info_us": `
SELECT
    "io" as measurement,
    devices.dev_ext_name,
    devices.dev_server_name,
    devices.dev_target_name,
    io_buckets.bucket_name,
    dev_types.dev_name,
    metering_info.interval,
    ROUND(CAST(metering_info.num_ios AS real) / metering_info.interval, 3) AS iops,
    ROUND(CAST(metering_info.bytes AS real) / metering_info.interval, 3) AS bps,
    ROUND(CAST((metering_info.total_resp_tm_us / 1000) AS real) / metering_info.num_ios, 3) AS latency_ms,
    metering_info.active_ios,
    metering_info.io_errors,
    metering_info.max_cmd,
    ROUND(CAST((metering_info.max_resp_tm_us / 1000) AS real), 3) AS max_latency_ms,
    metering_info.time
FROM
    metering_info
    INNER JOIN
        devices
        ON (metering_info.dev_dbid = devices.dev_dbid)
    INNER JOIN
        io_buckets
        ON (devices.dev_type = io_buckets.dev_type)
        AND (metering_info.bucket = io_buckets.bucket)
    INNER JOIN
        dev_types
        ON (devices.dev_type = dev_types.dev_type)
        AND (dev_types.dev_type = io_buckets.dev_type)
ORDER BY
    metering_info.time
`,
		"metering_sys_info": `
SELECT
    "system" as measurement,
    dev_types.dev_name,
    metering_sys_info.interval,
    ROUND(CAST(100.0 * metering_sys_info.cpu_user AS real) / (metering_sys_info.cpu_user + metering_sys_info.cpu_system + metering_sys_info.cpu_iowait + metering_sys_info.cpu_idle), 3) AS cpu_user,
    ROUND(CAST(100.0 * metering_sys_info.cpu_system AS real) / (metering_sys_info.cpu_user + metering_sys_info.cpu_system + metering_sys_info.cpu_iowait + metering_sys_info.cpu_idle), 3) AS cpu_system,
    ROUND(CAST(100.0 * metering_sys_info.cpu_iowait AS real) / (metering_sys_info.cpu_user + metering_sys_info.cpu_system + metering_sys_info.cpu_iowait + metering_sys_info.cpu_idle), 3) AS cpu_iowait,
    ROUND(CAST(100.0 * metering_sys_info.cpu_idle AS real) / (metering_sys_info.cpu_user + metering_sys_info.cpu_system + metering_sys_info.cpu_iowait + metering_sys_info.cpu_idle), 3) AS cpu_idle,
    metering_sys_info.mem_alloc,
    metering_sys_info.time
FROM
    metering_sys_info
    INNER JOIN
        devices
        ON (metering_sys_info.dev_dbid = devices.dev_dbid)
    INNER JOIN
        dev_types
        ON (devices.dev_type = dev_types.dev_type)
WHERE
    devices.dev_type = 5
ORDER BY
    metering_sys_info.time
`,
		"metering_zcache_info": `
SELECT
    "zcache" as measurement,
    dev_types.dev_name,
    devices.dev_ext_name,
    metering_zcache_info.interval,
    metering_zcache_info.data_dirty,
    metering_zcache_info.meta_dirty,
    metering_zcache_info.data_clean,
    metering_zcache_info.meta_clean,
    metering_zcache_info.data_cb_util,
    metering_zcache_info.meta_cb_util,
    metering_zcache_info.data_read_hit,
    metering_zcache_info.meta_read_hit,
    metering_zcache_info.data_write_hit,
    metering_zcache_info.meta_write_hit,
    metering_zcache_info.time
FROM
    metering_zcache_info
    INNER JOIN
       devices
       ON (metering_zcache_info.dev_dbid = devices.dev_dbid)
    INNER JOIN
       dev_types
       ON (devices.dev_type = dev_types.dev_type)
WHERE
    devices.dev_type = 7
ORDER BY
    metering_zcache_info.time
`,
	}
)

type dataio struct {
	input   *sqlx.DB
	inputDB string
	output  client.Client
}

func newDataIO(inputDB string, output string) (conn *dataio, err error) {
	if _, err := os.Stat(inputDB); os.IsNotExist(err) {
		return nil, err
	}

	i, err := sqlx.Connect("sqlite3", inputDB)

	if err != nil {
		return nil, err
	}

	jww.DEBUG.Printf("successfully opened input dataio database at: %v", inputDB)

	o, err := client.NewHTTPClient(client.HTTPConfig{
		Addr: output,
	})

	if err != nil {
		return nil, err
	}

	jww.DEBUG.Printf("successfully opened output dataio database at: %v", output)

	return &dataio{i, inputDB, o}, nil
}
