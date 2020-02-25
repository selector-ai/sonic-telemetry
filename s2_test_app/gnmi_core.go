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
	rclient.FlushDB()
	//Enable keysapce notification
	os.Setenv("PATH", "/usr/bin:/sbin:/bin:/usr/local/bin")
	cmd := exec.Command("redis-cli", "config", "set", "notify-keyspace-events", "KEA")
	_, err := cmd.Output()
	if err != nil {
		log.Fatal("failed to enable redis keyspace notification ", err)
	}

	fileName := "../s2_test_data/COUNTERS_PORT_NAME_MAP.txt"
	countersPortNameMapByte, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Fatalf("read file %v err: %v", fileName, err)
	}
	mpi_name_map := loadConfig("COUNTERS_PORT_NAME_MAP", countersPortNameMapByte)
	loadDB(rclient, mpi_name_map)

	fileName = "../s2_test_data/COUNTERS_QUEUE_NAME_MAP.txt"
	countersQueueNameMapByte, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Fatalf("read file %v err: %v", fileName, err)
	}
	mpi_qname_map := loadConfig("COUNTERS_QUEUE_NAME_MAP", countersQueueNameMapByte)
	loadDB(rclient, mpi_qname_map)

	fileName = "../s2_test_data/COUNTERS:Ethernet0.txt"
	countersEthernet0Byte, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Fatalf("read file %v err: %v", fileName, err)
	}
	// "Ethernet0": "oid:0x15000000000025",
	mpi_counter := loadConfig("COUNTERS:oid:0x15000000000025", countersEthernet0Byte)
	loadDB(rclient, mpi_counter)

	fileName = "../s2_test_data/COUNTERS:Ethernet8.txt"
	countersEthernet8Byte, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Fatalf("read file %v err: %v", fileName, err)
	}
	// "Ethernet8": "oid:0x1500000000004d",
	mpi_counter = loadConfig("COUNTERS:oid:0x1500000000004d", countersEthernet8Byte)
	loadDB(rclient, mpi_counter)

	fileName = "../s2_test_data/COUNTERS:Ethernet68.txt"
	countersEthernet68Byte, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Fatalf("read file %v err: %v", fileName, err)
	}
	// "Ethernet68": "oid:0x15000000000179",
	mpi_counter = loadConfig("COUNTERS:oid:0x15000000000179", countersEthernet68Byte)
	loadDB(rclient, mpi_counter)

	// Load CONFIG_DB for alias translation
	//prepareConfigDb(t)
}

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

//This function takes the xpath and derives the right file and modifies the counter.
//After that it will be programmed either into redis or using telemetry app via gnmi server
func setCountersInDB(xpath string, countervalue int) {

	xpathElements, err := xpath.ParseStringPath(xpath)
	if err != nil {
		return nil, err
	}
        fileName := xpathElements["Counters"]
        countersPortAliasMapByte, err := ioutil.ReadFile(fileName)
        if err != nil {
                log.Println("read file %v err: %v", fileName, err)
        }
        mpi_alias_map := loadConfig("", countersPortAliasMapByte)
	batters := mpi_alias_map["Counters"].(map[string]interface{})["Ethernet"]

	ioutil.WriteFile(settingFile, out, 0600)

	//Program the new values - TBD
}

func InitSwitchStateInDB() {
	// Inform gNMI server to use redis tcp localhost connection
	sdc.UseRedisLocalTcpPort = true
	LoadBasicMapAndState()
}
