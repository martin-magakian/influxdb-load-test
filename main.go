package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"os"
	"runtime"
	"time"

	"github.com/influxdb/influxdb/client"
	"github.com/rcrowley/go-metrics"
)

func main() {

	loadTest := LoadTest{
		host:            flag.String("h", "localhost", "host"),
		port:            flag.Int("p", 8086, "port number"),
		db:              flag.String("db", "load_test", "database"),
		measurement:     flag.String("m", "load_test", "measurement"),
		retentionPolicy: flag.String("rp", "default", "retention policy"),
		batchSize:       flag.Int("batchSize", 5000, "batch size for requests"),
		rate:            flag.Int("rate", 5, "requests per second"),
		cpus:            flag.Int("cpus", runtime.NumCPU(), "Number of CPUs to use"),
		duration:        flag.Int("duration", 60, "time in seconds for test to run"),
		Logger:          log.New(os.Stderr, "main: ", log.Lmicroseconds),
	}

	flag.Parse()

	runtime.GOMAXPROCS(*loadTest.cpus)

	// Log the metrics at the end of the load test
	go metrics.Log(metrics.DefaultRegistry, time.Duration(*loadTest.duration)*time.Second, log.New(os.Stderr, "metrics: ", log.Lmicroseconds))

	loadTest.run()
}

// LoadTest configuration
type LoadTest struct {
	host            *string
	port            *int
	db              *string
	measurement     *string
	retentionPolicy *string
	batchSize       *int
	rate            *int
	cpus            *int
	duration        *int
	errorMeter      metrics.Meter
	Logger          *log.Logger
}

func (l *LoadTest) run() {
	l.Logger.Println("starting load test")

	u, _ := url.Parse(fmt.Sprintf("http://%s:%d", *l.host, *l.port))
	con, err := client.NewClient(client.Config{URL: *u})

	if err != nil {
		panic(err)
	}

	createDatabase(con, l)

	durationCounter := 0

	t := metrics.NewTimer()
	metrics.Register("requests", t)

	l.errorMeter = metrics.NewMeter()
	metrics.Register("errorMeter", l.errorMeter)

	for _ = range time.Tick(time.Second) {
		if durationCounter >= *l.duration {
			return
		}

		l.Logger.Printf("sending more points...running for %d seconds", durationCounter)

		for i := 0; i < *l.rate; i++ {
			go func() {
				t.Time(func() {
					writePoints(con, l)
				})
			}()
		}
		durationCounter++
	}
}

func createDatabase(con *client.Client, l *LoadTest) {
	database := *l.db
	l.Logger.Printf("creating database %s, if doesn't already exist", database)

	q := client.Query{
		Command:  fmt.Sprintf("create database %s", database),
		Database: database,
	}

	if _, err := con.Query(q); err != nil {
		panic(err)
	}
}

func writePoints(con *client.Client, l *LoadTest) {
	var (
		hosts     = []string{"host1", "host2", "host3", "host4", "host5", "host6"}
		metrics   = []string{"com.addthis.Service.total._red_pjson__.1MinuteRate", "com.addthis.Service.total._red_lojson_100eng.json.1MinuteRate", "com.addthis.Service.total._red_lojson_300lo.json.1MinuteRate"}
		batchSize = *l.batchSize
		pts       = make([]client.Point, batchSize)
	)

	for i := 0; i < batchSize; i++ {
		pts[i] = client.Point{
			Measurement: *l.measurement,
			Tags: map[string]string{
				"host":   hosts[rand.Intn(len(hosts))],
				"metric": metrics[rand.Intn(len(metrics))],
			},
			Fields: map[string]interface{}{
				"value": rand.Float64(),
			},
			Time:      time.Now(),
			Precision: "n",
		}
	}

	bps := client.BatchPoints{
		Points:          pts,
		Database:        *l.db,
		RetentionPolicy: *l.retentionPolicy,
	}

	if _, err := con.Write(bps); err != nil {
		l.errorMeter.Mark(1)
		log.Fatal(err)
	}
}
