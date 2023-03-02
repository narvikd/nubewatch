package register

import (
	"encoding/json"
	"fmt"
	"github.com/hashicorp/raft"
	"github.com/narvikd/errorskit"
	"log"
	"nubewatch/cluster"
	"nubewatch/cluster/consensus/fsm"
	"nubewatch/pkg/httpclient"
	"nubewatch/register/registermodel"
	"sync"
	"time"
)

type State struct {
	sync.RWMutex
	initInProgress bool
}

func (s *State) IsInitInProgress() bool {
	s.RLock()
	defer s.RUnlock()
	return s.initInProgress
}

func (s *State) SetInitInProgress(b bool) {
	s.Lock()
	defer s.Unlock()
	s.initInProgress = b
}

func (s *State) CheckServers(consensus *raft.Raft, fsm *fsm.DatabaseFSM) {
	if s.IsInitInProgress() {
		log.Println("CheckServers already in progress")
		return
	}
	s.SetInitInProgress(true)
	defer s.SetInitInProgress(false)

	for {
		if consensus.State() != raft.Leader {
			log.Println("EXITING CHECKING. NOT A LEADER (CheckServers)")
			break
		}

		servers, errGetAllServers := fsm.GetAllServers()
		if errGetAllServers != nil {
			errorskit.LogWrap(errGetAllServers, "couldn't get servers to check if they are alive")
			continue
		}

		for _, server := range servers {
			go CheckServer(consensus, server)
		}
		break
	}
}

func CheckServer(consensus *raft.Raft, server registermodel.Server) {
	const (
		maxRetryCount    = 5
		sleepForEveryReq = 3 * time.Second
	)
	currentTry := 0
	currentSleep := 2 * time.Second
	u := fmt.Sprintf("%s://%s/%s", server.Protocol, server.Host, server.HealthCheckEndpoint)
	for {
		if consensus.State() != raft.Leader {
			log.Println("EXITING CHECKING. NOT A LEADER (CheckServer)")
			break
		}

		httpReq := httpclient.Request{
			Endpoint:   u,
			RestMethod: "GET",
			Timeout:    3 * time.Second,
		}
		res := httpReq.Do()
		if res.Err != nil {
			currentTry++
			log.Printf("server %s. Request failed attempt: %v. Err: %v\n", server.ID, currentTry, res.Err)

			if currentTry >= maxRetryCount {
				log.Printf("server %s. Retry max reached!. Setting server as offline.", server.ID)

				p := &fsm.Payload{
					Key:       server.ServiceName,
					Value:     server.ID,
					Operation: "SET_NOT_ALIVE_SERVER",
				}

				j, errMarshal := marshalPayload(p)
				if errMarshal != nil {
					log.Println(errMarshal)
					break
				}

				_, errNotAlive := cluster.ApplyLeaderFuture(consensus, j)
				if errNotAlive != nil {
					errorskit.LogWrap(errNotAlive, "COULDN'T SET SERVER TO NOT ALIVE SERVER AFTER TIMEOUT. ID: "+server.ID)
					break
				}
				break
			}

			log.Printf("server %s. request delayed for %v\n", server.ID, currentSleep)
			time.Sleep(currentSleep)
			currentSleep *= 2
		}

		p := &fsm.Payload{
			Key:       server.ServiceName,
			Value:     server,
			Operation: "UPDATE_LAST_CONTACT",
		}

		j, errMarshal := marshalPayload(p)
		if errMarshal != nil {
			log.Println(errMarshal)
			break
		}

		_, errLastUpdate := cluster.ApplyLeaderFuture(consensus, j)
		if errLastUpdate != nil {
			errorskit.LogWrap(errLastUpdate, "couldn't execute UPDATE_LAST_CONTACT")
		}

		time.Sleep(sleepForEveryReq)
	}
}

func marshalPayload(payload *fsm.Payload) ([]byte, error) {
	payloadData, errMarshal := json.Marshal(&payload)
	if errMarshal != nil {
		return nil, errorskit.Wrap(errMarshal, "couldn't marshal data")
	}
	return payloadData, nil
}
