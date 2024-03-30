package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/denisdubovitskiy/findmetrics/internal/finder"
)

func main() {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("unable to determine a working directory: %v", err)
	}

	metrics, err := finder.FindPrometheusMetrics(wd)
	if err != nil {
		log.Fatalf("unable to find metrics: %v", err)
	}

	for _, metric := range metrics {
		bytes, err := json.Marshal(metric)
		if err != nil {
			log.Fatalf("unable to marshal JSON: %v", err)
		}
		fmt.Println(string(bytes))
	}
}
