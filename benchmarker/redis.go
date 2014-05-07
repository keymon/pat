package benchmarker

import (
	"encoding/json"
	"net/url"
"fmt"	
	"github.com/cloudfoundry-incubator/pat/context"
	"github.com/cloudfoundry-incubator/pat/logs"
	"github.com/cloudfoundry-incubator/pat/redis"
	"github.com/cloudfoundry-incubator/pat/workloads"
	"github.com/nu7hatch/gouuid"
)

type rw struct {
	defaultWorker
	conn             redis.Conn
	timeoutInSeconds int
}

type RedisMessage struct { 
	Reply string
	Workload string
	WorkloadContext context.WorkloadContext
}

const DefaultTimeout = 60 * 5

func NewRedisWorker(conn redis.Conn) Worker {
	return NewRedisWorkerWithTimeout(conn, DefaultTimeout)
}

func NewRedisWorkerWithTimeout(conn redis.Conn, timeoutInSeconds int) Worker {
	return &rw{defaultWorker{make(map[string]workloads.WorkloadStep)}, conn, timeoutInSeconds}
}

func (rw rw) Time(workload string, workloadCtx context.WorkloadContext) (result IterationResult) {
	
	guid, _ := uuid.NewV4()	
	//workloadCtx = replaceSpaceWithEscape(workloadCtx)
	//jsonContent, jsonErr := workloadCtx.MarshalJSON()

	// if jsonErr != nil {
	// 	return IterationResult{0, []StepResult{}, encodeError(jsonErr)}
	// } 
fmt.Printf("Marshal key: %t\n", workloadCtx.CheckExists("workerIndex"))

//fmt.Println("content:"+string(workloadCtx.GetContentJSON()))
	redisMsg := RedisMessage{
		Workload: workload,
		Reply: "replies-"+guid.String(),
		WorkloadContext: workloadCtx,		
	}
	var jsonRedisMsg []byte

	jsonRedisMsg, _ = json.Marshal(redisMsg)
fmt.Println(string(jsonRedisMsg))
	rw.conn.Do("RPUSH", "tasks", string(jsonRedisMsg))

	reply, err := redis.Strings(rw.conn.Do("BLPOP", "replies-"+guid.String(), rw.timeoutInSeconds))
	if err != nil {
		return IterationResult{0, []StepResult{}, encodeError(err)}
	} else {
		json.Unmarshal([]byte(reply[1]), &result)
		return
	}
}

type slave struct {
	guid string
	conn redis.Conn
}

func StartSlave(conn redis.Conn, delegate Worker) slave {
	guid, _ := uuid.NewV4()
	go slaveLoop(conn, delegate, guid.String())
	return slave{guid.String(), conn}
}

func (slave slave) Close() error {
	_, err := slave.conn.Do("RPUSH", "stop-"+slave.guid, true)
	if err == nil {
		_, err = slave.conn.Do("BLPOP", "stopped-"+slave.guid, DefaultTimeout)
	}

	logs.NewLogger("redis.slave").Infof("Redis slave shutting down, %v", err)
	return err
}

func slaveLoop(conn redis.Conn, delegate Worker, handle string) {
	//var tmpContent context.WorkloadContent
	logger := logs.NewLogger("redis.slave")
	logger.Info("Started slave")

	for {
		reply, err := redis.Strings(conn.Do("BLPOP", "stop-"+handle, "tasks", 0))

		if len(reply) == 0 {
			panic("Empty task, usually means connection lost, shutting down")
		}

		if reply[0] == "stop-"+handle {
			conn.Do("RPUSH", "stopped-"+handle, true)
			break
		}

		if err == nil {
			var redisMsg RedisMessage
		fmt.Println("before Unmarshal")
			json.Unmarshal([]byte(reply[1]), &redisMsg)			
			
			fmt.Println("1 Unmarshal")

			//json.Unmarshal(redisMsg.WorkloadContext, &tmpContext )
			//redisMsg.WorkloadContext.Unmarshal(redisMsg.WorkloadContext)
			fmt.Println("2 Unmarshal")
			//var workloadCtx = context.NewWithContent( tmpContext )
			fmt.Println(string(reply[1]))
			var workloadCtx = redisMsg.WorkloadContext
			fmt.Printf("got over assigning: \n")
			fmt.Printf("3 Unmarshal key: %t\n", workloadCtx.CheckExists("workerIndex"))
			


			//workloadCtx = UnescapeString(workloadCtx)
			go func(experiment string, replyTo string, workloadCtx context.WorkloadContext) {								
				result := delegate.Time(experiment, workloadCtx)				
				var encoded []byte
				encoded, err = json.Marshal(result)				
				logger.Debug("Completed slave task, replying")
				conn.Do("RPUSH", replyTo, string(encoded))
			}(redisMsg.Workload, redisMsg.Reply, workloadCtx)

		}

		if err != nil {
			logger.Warnf("ERROR: slave encountered error: %v", err)
		}
	}
}

func replaceSpaceWithEscape(workloadCtx context.WorkloadContext) context.WorkloadContext {
	for _, k := range workloadCtx.GetKeys() {		
		if workloadCtx.CheckType(k) == "string" {
			workloadCtx.PutString( k, url.QueryEscape(workloadCtx.GetString(k)) )
		}
	}
	return workloadCtx
}

func UnescapeString(workloadCtx context.WorkloadContext) context.WorkloadContext {
	var str string
	for _, k := range workloadCtx.GetKeys() {
		if workloadCtx.CheckType(k) == "string" {
			str, _ =url.QueryUnescape(workloadCtx.GetString(k))
			workloadCtx.PutString(k, str)
		}
	}
	return workloadCtx
}
