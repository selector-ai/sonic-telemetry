package main

import (
	"log"
	"time"
	"sync"
	"crypto/rand"
	"math/big"

	"github.com/BurntSushi/toml"
)

//Entire counter configuration
type sonicCounterConfig struct {
	Title   string
	App     appInfo
	Counters map[string]counter
}

//Sonic related information
type appInfo struct {
	Name string
}

//Description of counter and its attributes
type counter struct {
	Description  string
	Counter_path string
	Counter_type string
	Counter_value int
	Interval_sec int
	Start_count  int
	Step_count   int
}

//Range specification, note that min <= max
type IntRange struct {
    min, max int
}

// get next random value within the interval including min and max
/*
func (ir *IntRange) NextRandom(r* rand.Rand) int {
    return r.Intn(ir.max - ir.min +1) + ir.min
}
*/

func processCountersRandom( data counter) {
	log.Println("Inside processCountersRandom")
	log.Printf("Values: (%s, %d, %d, %d, %d)\n", data.Counter_type, data.Counter_value, data.Interval_sec, data.Start_count, data.Step_count)
	c1 := make(chan int64, 1)
	for {
		go func() {
			log.Println("Sleeping for rand-interval", time.Duration(data.Interval_sec))
			time.Sleep(time.Duration(data.Interval_sec) * time.Second)
			//rand.Seed(int64(time.Now().Nanosecond()))
			//r2 := rand.New(rand.NewSource(42))
			//r2 := rand.Int()
			r2, _ := rand.Int(rand.Reader, big.NewInt(1000))
			//ir := IntRange{-1,1}
			//log.Println(ir.NextRandom(r2))
			//randomVal := r2.Intn(9999)
			randomVal := r2.Int64()
			log.Print("Random: ", randomVal, ",")
			c1 <- randomVal
		}()

		select {
		case res := <-c1:
			log.Println(res)
			setCountersInDB(data.Counter_path, data.Counter_value)
		case <-time.After(15 * time.Second):
			log.Println("timeout 1")
		}
	}
}

func processCountersIncrement( data counter) {
	log.Println("Inside processCountersIncrement")
	log.Printf("Values: (%s, %d, %d, %d, %d)\n", data.Counter_type, data.Counter_value, data.Interval_sec, data.Start_count, data.Step_count)
	c1 := make(chan int, 1)
	n := data.Start_count
	for {
		go func() {
			log.Println("Sleeping for incr-interval", time.Duration(data.Interval_sec))
			time.Sleep(time.Duration(data.Interval_sec) * time.Second)
			newVal := n + data.Step_count
			log.Print("Increment: ", newVal, ",")
			c1 <- newVal
		}()

		select {
		case res := <-c1:
			n = res
			log.Println(res)
			setCountersInDB(data.Counter_path, data.Counter_value)
		case <-time.After(15 * time.Second):
			log.Println("timeout 1")
		}
	}
}

func processCountersFixed( data counter) {
	log.Println("Inside processCountersFixed")
	log.Printf("Values: (%s, %d, %d, %d, %d)\n", data.Counter_type, data.Counter_value, data.Interval_sec, data.Start_count, data.Step_count)
	setCountersInDB(data.Counter_path, data.Counter_value)
}

func processCounters(wg *sync.WaitGroup, id int, counterName string, data counter) {
	defer wg.Done()

	log.Printf("Worker %v: Started\n", id)
	//time.Sleep(time.Second)
	log.Printf("Counter: %s (%s, %s)\n", counterName, data.Description, data.Counter_path)

	if data.Counter_type == "fixed" {
		processCountersFixed(data)
	} else if data.Counter_type == "incrementing" {
		processCountersIncrement(data)
	} else if data.Counter_type == "random" {
		processCountersRandom(data)
	} else {
		log.Printf("Unknown type")
	}

	log.Printf("Worker %v: Finished\n", id)
}


func main() {
	//First, initialize the counter DB state
	InitSwitchStateInDB()
	//Now read the policy file and create counter state
	var wg sync.WaitGroup
	var config sonicCounterConfig

	if _, err := toml.DecodeFile("/sonic/input/policy.toml", &config); err != nil {
		log.Print(err)
		return
	}

	//Print some useful values from policy file
	log.Printf("Config: %v\n", config)
	log.Printf("Title: %s\n", config.Title)
	log.Printf("App: %s \n", config.App.Name)

	i := 0
	for counterName, counter := range config.Counters {
		log.Println("test-app main: Starting worker", i)
		wg.Add(1)
		i++
		go processCounters(&wg, i, counterName, counter)
	}

	//Wait until all threads are complete
	log.Println("test-app main: Waiting for workers to finish")
	wg.Wait()
	log.Println("test-app: exiting")
}
