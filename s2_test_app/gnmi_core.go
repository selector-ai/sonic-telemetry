package main

// server_test covers gNMI get, subscribe (stream and poll) test
// Prerequisite: redis-server should be running.

import (
	"crypto/tls"
	"encoding/json"
	"github.com/go-redis/redis"
	"github.com/golang/protobuf/proto"

	//"github.com/kylelemons/godebug/pretty"
	//"github.com/openconfig/gnmi/client"
	pb "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/gnmi/value"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"io/ioutil"
	"os"
	"os/exec"
	"reflect"
	"time"
	"log"
	// Register supported client types.
	sdc "github.com/Azure/sonic-telemetry/sonic_data_client"
	sdcfg "github.com/Azure/sonic-telemetry/sonic_db_config"
	gclient "github.com/jipanyang/gnmi/client/gnmi"
	"github.com/google/gnxi/utils/xpath"
)

var clientTypes = []string{gclient.Type}

func loadConfig(key string, in []byte) map[string]interface{} {
	var fvp map[string]interface{}

	err := json.Unmarshal(in, &fvp)
	if err != nil {
		log.Println("Failed to Unmarshal %v err: %v", in, err)
	}
	if key != "" {
		kv := map[string]interface{}{}
		kv[key] = fvp
		return kv
	}
	return fvp
}

// assuming input data is in key field/value pair format
func loadDB(rclient *redis.Client, mpi map[string]interface{}) {
	for key, fv := range mpi {
		switch fv.(type) {
		case map[string]interface{}:
			_, err := rclient.HMSet(key, fv.(map[string]interface{})).Result()
			if err != nil {
				log.Println("Invalid data for db:  %v : %v %v", key, fv, err)
			}
		default:
			log.Println("Invalid data for db: %v : %v", key, fv)
		}
	}
}

// runTestGet requests a path from the server by Get grpc call, and compares if
// the return code and response value are expected.
func runTestGet(ctx context.Context, gClient pb.GNMIClient, pathTarget string,
	textPbPath string, wantRetCode codes.Code, wantRespVal interface{}) {

	// Send request
	var pbPath pb.Path
	if err := proto.UnmarshalText(textPbPath, &pbPath); err != nil {
		log.Fatalf("error in unmarshaling path: %v %v", textPbPath, err)
	}
	prefix := pb.Path{Target: pathTarget}
	req := &pb.GetRequest{
		Prefix:   &prefix,
		Path:     []*pb.Path{&pbPath},
		Encoding: pb.Encoding_JSON_IETF,
	}

	resp, err := gClient.Get(ctx, req)
	// Check return code
	gotRetStatus, ok := status.FromError(err)
	if !ok {
		log.Fatal("got a non-grpc error from grpc call")
	}
	if gotRetStatus.Code() != wantRetCode {
		log.Println("err: ", err)
		log.Fatalf("got return code %v, want %v", gotRetStatus.Code(), wantRetCode)
	}

	// Check response value
	var gotVal interface{}
	if resp != nil {
		notifs := resp.GetNotification()
		if len(notifs) != 1 {
			log.Fatalf("got %d notifications, want 1", len(notifs))
		}
		updates := notifs[0].GetUpdate()
		if len(updates) != 1 {
			log.Fatalf("got %d updates in the notification, want 1", len(updates))
		}
		val := updates[0].GetVal()
		if val.GetJsonIetfVal() == nil {
			gotVal, err = value.ToScalar(val)
			if err != nil {
				log.Println("got: %v, want a scalar value", gotVal)
			}
		} else {
			// Unmarshal json data to gotVal container for comparison
			if err := json.Unmarshal(val.GetJsonIetfVal(), &gotVal); err != nil {
				log.Fatalf("error in unmarshaling IETF JSON data to json container: %v", err)
			}
			var wantJSONStruct interface{}
			if err := json.Unmarshal(wantRespVal.([]byte), &wantJSONStruct); err != nil {
				log.Fatalf("error in unmarshaling IETF JSON data to json container: %v", err)
			}
			wantRespVal = wantJSONStruct
		}
	}

	if !reflect.DeepEqual(gotVal, wantRespVal) {
		log.Println("got: %v (%T),\nwant %v (%T)", gotVal, gotVal, wantRespVal, wantRespVal)
	}
}

func getRedisClient() *redis.Client {
	rclient := redis.NewClient(&redis.Options{
		Network:     "tcp",
		Addr:        sdcfg.GetDbTcpAddr("COUNTERS_DB"),
		Password:    "", // no password set
		DB:          sdcfg.GetDbId("COUNTERS_DB"),
		DialTimeout: 0,
	})
	_, err := rclient.Ping().Result()
	if err != nil {
		log.Fatalf("failed to connect to redis server %v", err)
	}
	return rclient
}

func getConfigDbClient() *redis.Client {
	rclient := redis.NewClient(&redis.Options{
		Network:     "tcp",
		Addr:        sdcfg.GetDbTcpAddr("CONFIG_DB"),
		Password:    "", // no password set
		DB:          sdcfg.GetDbId("CONFIG_DB"),
		DialTimeout: 0,
	})
	_, err := rclient.Ping().Result()
	if err != nil {
		log.Fatalf("failed to connect to redis server %v", err)
	}
	return rclient
}

func loadConfigDB(rclient *redis.Client, mpi map[string]interface{}) {
	for key, fv := range mpi {
		switch fv.(type) {
		case map[string]interface{}:
			_, err := rclient.HMSet(key, fv.(map[string]interface{})).Result()
			if err != nil {
				log.Println("Invalid data for db: %v : %v %v", key, fv, err)
			}
		default:
			log.Println("Invalid data for db: %v : %v", key, fv)
		}
	}
}

func prepareDb() {
	rclient := getRedisClient()
	defer rclient.Close()
	//Enable keysapce notification
	os.Setenv("PATH", "/usr/bin:/sbin:/bin:/usr/local/bin")
	cmd := exec.Command("redis-cli", "config", "set", "notify-keyspace-events", "KEA")
	_, err := cmd.Output()
	if err != nil {
		log.Fatal("failed to enable redis keyspace notification ", err)
	}

	fileName := "../s2_test_data/COUNTERS:Ethernet0.txt"
	countersEthernet0, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Fatalf("read file %v err: %v", fileName, err)
	}
	// "Ethernet0": "oid:0x1000000000002", for port based
	port_counter := loadConfig("COUNTERS:oid:0x1000000000002", countersEthernet0)
	loadDB(rclient, port_counter)

	fileName = "../s2_test_data/COUNTERS:Ethernet8.txt"
	countersEthernet8, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Fatalf("read file %v err: %v", fileName, err)
	}
	// "Ethernet8": "oid:0x1000000000004", for port based
	port_counter = loadConfig("COUNTERS:oid:0x1000000000004", countersEthernet8)
	loadDB(rclient, port_counter)

	fileName = "../s2_test_data/COUNTERS:Ethernet68.txt"
	countersEthernet68, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Fatalf("read file %v err: %v", fileName, err)
	}
	// "Ethernet68": "oid:0x1000000000013", for port based
	port_counter = loadConfig("COUNTERS:oid:0x1000000000013", countersEthernet68)
	loadDB(rclient, port_counter)

	//Now fill the queue based counters
	fileName = "../s2_test_data/COUNTERS:Ethernet0:Queues.txt"
	countersEthernet0Byte, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Fatalf("read file %v err: %v", fileName, err)
	}
	// "Ethernet0": "oid:0x15000000000025", for queue based
	mpi_counter := loadConfig("COUNTERS:oid:0x15000000000025", countersEthernet0Byte)
	loadDB(rclient, mpi_counter)

	fileName = "../s2_test_data/COUNTERS:Ethernet8:Queues.txt"
	countersEthernet8Byte, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Fatalf("read file %v err: %v", fileName, err)
	}
	// "Ethernet8": "oid:0x1500000000004d", for queue based
	mpi_counter = loadConfig("COUNTERS:oid:0x1500000000004d", countersEthernet8Byte)
	loadDB(rclient, mpi_counter)

	fileName = "../s2_test_data/COUNTERS:Ethernet68:Queues.txt"
	countersEthernet68Byte, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Fatalf("read file %v err: %v", fileName, err)
	}
	// "Ethernet68": "oid:0x15000000000179", for queue based
	mpi_counter = loadConfig("COUNTERS:oid:0x15000000000179", countersEthernet68Byte)
	loadDB(rclient, mpi_counter)

}

//Function to load basic port map and queue state through GNMI
func LoadBasicMapAndState() {

	//Prepare the config to update the database
	prepareDb()

	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	opts := []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))}

	targetAddr := "127.0.0.1:8080"
	conn, err := grpc.Dial(targetAddr, opts...)
	if err != nil {
		log.Fatalf("Dialing to %q failed: %v", targetAddr, err)
	}
	defer conn.Close()

	gClient := pb.NewGNMIClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Load the COUNTERS_PORT_NAME_MAP
	fileName := "../s2_test_data/COUNTERS_PORT_NAME_MAP.txt"
	countersPortNameMapByte, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Fatalf("read file %v err: %v", fileName, err)
	}

	// Load the COUNTERS:Ethernet0 counters
	fileName = "../s2_test_data/COUNTERS:Ethernet0.txt"
	countersEthernet0Byte, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Fatalf("read file %v err: %v", fileName, err)
	}

	fileName = "../s2_test_data/COUNTERS:Ethernet8.txt"
	countersEthernet8Byte, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Fatalf("read file %v err: %v", fileName, err)
	}

	fileName = "../s2_test_data/COUNTERS:Ethernet68.txt"
	countersEthernet68Byte, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Fatalf("read file %v err: %v", fileName, err)
	}

	tds := []struct {
		desc        string
		pathTarget  string
		textPbPath  string
		wantRetCode codes.Code
		wantRespVal interface{}
	}{{
		desc:       "Test non-existing path Target",
		pathTarget: "MY_DB",
		textPbPath: `
			elem: <name: "MyCounters" >
		`,
		wantRetCode: codes.NotFound,
	}, {
		desc:       "Test empty path target",
		pathTarget: "",
		textPbPath: `
			elem: <name: "MyCounters" >
		`,
		wantRetCode: codes.Unimplemented,
	}, {
		desc:       "Get valid but non-existing node",
		pathTarget: "COUNTERS_DB",
		textPbPath: `
			elem: <name: "MyCounters" >
		`,
		wantRetCode: codes.NotFound,
	}, {
		desc:       "Get COUNTERS_PORT_NAME_MAP",
		pathTarget: "COUNTERS_DB",
		textPbPath: `
			elem: <name: "COUNTERS_PORT_NAME_MAP" >
		`,
		wantRetCode: codes.OK,
		wantRespVal: countersPortNameMapByte,
	}, {
		desc:       "get COUNTERS:Ethernet0",
		pathTarget: "COUNTERS_DB",
		textPbPath: `
					elem: <name: "COUNTERS" >
					elem: <name: "Ethernet0" >
				`,
		wantRetCode: codes.OK,
		wantRespVal: countersEthernet0Byte,
	}, {
		desc:       "get COUNTERS:Ethernet8",
		pathTarget: "COUNTERS_DB",
		textPbPath: `
					elem: <name: "COUNTERS" >
					elem: <name: "Ethernet8" >
				`,
		wantRetCode: codes.OK,
		wantRespVal: countersEthernet8Byte,
	}, {
		desc:       "get COUNTERS:Ethernet68",
		pathTarget: "COUNTERS_DB",
		textPbPath: `
					elem: <name: "COUNTERS" >
					elem: <name: "Ethernet68" >
				`,
		wantRetCode: codes.OK,
		wantRespVal: countersEthernet68Byte,
	}, {
		desc:       "get COUNTERS:Ethernet68 SAI_PORT_STAT_PFC_7_RX_PKTS",
		pathTarget: "COUNTERS_DB",
		textPbPath: `
					elem: <name: "COUNTERS" >
					elem: <name: "Ethernet68" >
					elem: <name: "SAI_PORT_STAT_PFC_7_RX_PKTS" >
				`,
		wantRetCode: codes.OK,
		wantRespVal: "6",
	}, {
		desc:       "get COUNTERS:Ethernet68:Queues",
		pathTarget: "COUNTERS_DB",
		textPbPath: `
					elem: <name: "COUNTERS" >
					elem: <name: "Ethernet68" >
				`,
		wantRetCode: codes.OK,
		wantRespVal: countersEthernet68Byte,
	}, {
		desc:       "get COUNTERS:Ethernet68 SAI_PORT_STAT_PFC_7_RX_PKTS",
		pathTarget: "COUNTERS_DB",
		textPbPath: `
					elem: <name: "COUNTERS" >
					elem: <name: "Ethernet68" >
					elem: <name: "SAI_PORT_STAT_PFC_7_RX_PKTS" >
				`,
		wantRetCode: codes.OK,
		wantRespVal: "6",
	}}

	for _, td := range tds {
		runTestGet(ctx, gClient, td.pathTarget, td.textPbPath, td.wantRetCode, td.wantRespVal)
	}
}

type tablePathValue struct {
	dbName    string
	tableName string
	tableKey  string
	delimitor string
	field     string
	value     string
	op        string
}

//Fetches the OID for a given port or the queue
func getCounterOID(counterType string, key string) string {
        fileName := "../s2_test_data/COUNTERS_PORT_NAME_MAP.txt"
        if counterType == "Queue" {
                fileName = "../s2_test_data/COUNTERS_QUEUE_NAME_MAP.txt"
        }
        countersEthernet0, err := ioutil.ReadFile(fileName)
        if err != nil {
                log.Fatalf("read file %v err: %v", fileName, err)
        }
        port_counter := loadConfig("", countersEthernet0)
        for a, k := range port_counter {
                //log.Println("Key:", a, "Value:", k)
                if a == key {
                        log.Println("Found key", k.(string))
                        return k.(string)
                }
        }
        return ""
}

func prepareCounterDb(workerId int, xpathVal string, counterVal int) (string, string) {
	//var m map[int]string
	//xpathVal := "/Counters/Ethernet8/Queues/SAI_QUEUE_STAT_CURR_OCCUPANCY_BYTES"
	//xpathVal := "/Counters/Ethernet8/SAI_PORT_STAT_PFC_7_RX_PKTS"
        xpathElements, err := xpath.ParseStringPath(xpathVal)
        if err != nil {
                return "",""
        }
	log.Println("Worker:", workerId, xpathElements)
	//counterVal := 100
	counterType := "Port"
	etherVal := xpathElements[1].(string)
	log.Println("Worker:", workerId, "Ethername is", etherVal)

	fileName := "../s2_test_data/COUNTERS:EthernetXX.txt"
	for _, bb := range xpathElements {
		//log.Println("NKey:", aa, "NValue:", bb)
		if bb == "Queues" {
			fileName = "../s2_test_data/COUNTERS:EthernetXX:Queues.txt"
			counterType = "Queue"
			//For now update only the first queue
			etherVal += ":0"
		}
	}
	lastElem := len(xpathElements) - 1
	log.Println("Worker:", workerId, xpathElements[lastElem])

	keyVal := xpathElements[lastElem]

	log.Println("Worker:", workerId, "Counter type", counterType, "Name", etherVal)
	oidVal := "COUNTERS:" + getCounterOID(counterType, etherVal)

        countersEthernet0, err := ioutil.ReadFile(fileName)
        if err != nil {
		log.Fatalf("Worker:", workerId, "read file %v err: %v", fileName, err)
        }
        port_counter := loadConfig("", countersEthernet0)
	for a, k := range port_counter {
		if a == keyVal {
			port_counter[a] = counterVal
			log.Println("Worker:", workerId, "Key:", a, "Field:", k, "Value", port_counter[a])
		} else {
			port_counter[a] = 0
		}
	}

	jsonString, err := json.Marshal(port_counter)
        if err != nil {
		log.Fatalf("Worker:", workerId, "marshalling error err: %v", err)
        }
	counterFileName := "../s2_test_data/" + oidVal
	log.Println("Worker:", workerId, "OID is", oidVal, "and filename is", counterFileName)
	ioutil.WriteFile(counterFileName, jsonString, os.ModePerm)

	return counterFileName, oidVal
}

//This function takes the xpath and derives the right file and modifies the counter.
//After that it will be programmed either into redis or using telemetry app via gnmi server
func SetCountersInDB(workerId int, xpath string, countervalue int) {

	rclient := getRedisClient()
	defer rclient.Close()
	//Enable keysapce notification
	os.Setenv("PATH", "/usr/bin:/sbin:/bin:/usr/local/bin")
	cmd := exec.Command("redis-cli", "config", "set", "notify-keyspace-events", "KEA")
	_, err := cmd.Output()
	if err != nil {
		log.Fatal("failed to enable redis keyspace notification ", err)
	}

	fileName , counterOID := prepareCounterDb(workerId, xpath, countervalue)
	countersEthernet0, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Fatalf("read file %v err: %v", fileName, err)
	}
	port_counter := loadConfig(counterOID, countersEthernet0)
	loadDB(rclient, port_counter)

}

func InitSwitchStateInDB() {
	// Inform gNMI server to use redis tcp localhost connection
	sdc.UseRedisLocalTcpPort = true
	LoadBasicMapAndState()
}
