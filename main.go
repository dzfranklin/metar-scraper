package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"
)

func main() {
	influxHost := mustGetEnv("INFLUX_HOST")
	influxToken := mustGetEnv("INFLUX_TOKEN")
	influxOrg := mustGetEnv("INFLUX_ORG")
	influxBucket := mustGetEnv("INFLUX_BUCKET")

	influxClient := influxdb2.NewClient(influxHost, influxToken)
	influxWrite := influxClient.WriteAPIBlocking(influxOrg, influxBucket)

	var lastObs time.Time
	for {
		slog.Info("waiting...")
		time.Sleep(time.Minute * 10)

		data, err := requestData()
		if err != nil {
			slog.Error("failed to request data", "error", err)
			continue
		}
		obsTime := time.Unix(int64(data.ObsTime), 0)
		slog.Info("got data", "data", data)

		if obsTime.After(lastObs) {
			slog.Info("data is new observation",
				"obsTime", obsTime, "prevObsTime", lastObs, "data", data)
			lastObs = obsTime

			point := write.NewPoint(
				"metar",
				map[string]string{"station": data.IcaoID},
				nil,
				obsTime,
			)

			if data.Temp != nil {
				point.AddField("temp", *data.Temp)
			}
			if data.Dewp != nil {
				point.AddField("dewp", *data.Dewp)
			}
			if data.Wdir != nil {
				point.AddField("wdir", *data.Wdir)
			}
			if data.Wspd != nil {
				point.AddField("wspd", *data.Wspd)
			}
			if data.Wgst != nil {
				point.AddField("wgst", *data.Wgst)
			}

			if err := influxWrite.WritePoint(context.Background(), point); err != nil {
				slog.Error("failed to write point", "error", err)
				continue
			}
			slog.Info("wrote point", "point", point)
		}
	}
}

type datapoint struct {
	ObsTime int    `json:"obsTime"`
	IcaoID  string `json:"icaoId"`
	Temp    *int   `json:"temp"`
	Dewp    *int   `json:"dewp"`
	Wdir    *int   `json:"wdir"`
	Wspd    *int   `json:"wspd"`
	Wgst    *int   `json:"wgst"`
}

func requestData() (*datapoint, error) {
	req, err := http.NewRequest("GET", "https://aviationweather.gov/api/data/metar?ids=EGPH&format=json", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "github.com/dzfranklin/metar-scraper (daniel@danielzfranklin.org)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status code %d", resp.StatusCode)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	var datapoints []datapoint
	if err := json.Unmarshal(respBody, &datapoints); err != nil {
		return nil, err
	}

	if len(datapoints) == 0 {
		return nil, fmt.Errorf("no datapoints found")
	}
	return &datapoints[0], nil
}

func mustGetEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("Missing required environment variable: %s", key)
	}
	return val
}
