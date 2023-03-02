// Package cluster is responsible for doing operations on the cluster.
package cluster

import (
	"encoding/json"
	"errors"
	"github.com/hashicorp/raft"
	"github.com/narvikd/errorskit"
	"log"
	"nubewatch/api/proto"
	"nubewatch/api/proto/protoclient"
	"nubewatch/cluster/consensus/fsm"
	"nubewatch/discover"
	"time"
)

const (
	errDBCluster      = "consensus returned an error when trying to Apply an order."
	errGrpcTalkLeader = "failed to get an ok response from the Leader via grpc"
)

func Execute(consensus *raft.Raft, payload *fsm.Payload) (any, error) {
	payloadData, errMarshal := json.Marshal(&payload)
	if errMarshal != nil {
		return nil, errorskit.Wrap(errMarshal, "couldn't marshal data to send it to the DB cluster")
	}

	if consensus.State() != raft.Leader {
		return forwardLeaderFuture(consensus, payload)
	}

	return ApplyLeaderFuture(consensus, payloadData)
}

// ApplyLeaderFuture applies a command on the Leader of the cluster.
//
// Should only be executed if the Node is a Leader.
func ApplyLeaderFuture(consensus *raft.Raft, payloadData []byte) (any, error) {
	const timeout = 500 * time.Millisecond

	if consensus.State() != raft.Leader {
		return nil, errors.New("node is not a leader")
	}

	future := consensus.Apply(payloadData, timeout)
	if future.Error() != nil {
		return nil, errorskit.Wrap(future.Error(), errDBCluster+" At future")
	}

	response := future.Response().(*fsm.ApplyRes)
	if response.Error != nil {
		return nil, errorskit.Wrap(response.Error, errDBCluster+" At response")
	}

	return response.Data, nil
}

func forwardLeaderFuture(consensus *raft.Raft, payload *fsm.Payload) (any, error) {
	var data any
	_, leaderID := consensus.LeaderWithID()
	if string(leaderID) == "" {
		return nil, errors.New("leader id was empty")
	}

	leaderGrpcAddr := discover.NewGrpcAddress(string(leaderID))
	log.Printf("[proto] payload for leader received in this node, forwarding to leader '%s' @ '%s'\n",
		leaderID, leaderGrpcAddr,
	)

	payloadData, errMarshal := json.Marshal(&payload)
	if errMarshal != nil {
		return nil, errorskit.Wrap(errMarshal, "couldn't marshal data to send it to the Leader's DB cluster")
	}

	conn, errConn := protoclient.NewConnection(leaderGrpcAddr)
	if errConn != nil {
		return nil, errConn
	}
	defer conn.Cleanup()

	response, errTalk := conn.Client.ExecuteOnLeader(conn.Ctx, &proto.ExecuteOnLeaderMsg{
		Payload: payloadData,
	})
	if errTalk != nil {
		return nil, errorskit.Wrap(errTalk, errGrpcTalkLeader)
	}

	if data != nil {
		errUnmarshal := json.Unmarshal(response.Payload, data)
		if errUnmarshal != nil {
			return nil, errorskit.Wrap(errUnmarshal, "couldn't unmarshal ExecuteOnLeader response")
		}
	}

	return data, nil
}

func ConsensusJoin(nodeID string, nodeConsensusAddr string, leaderGrpcAddr string) error {
	conn, errConn := protoclient.NewConnection(leaderGrpcAddr)
	if errConn != nil {
		return errConn
	}
	defer conn.Cleanup()

	_, errTalk := conn.Client.ConsensusJoin(conn.Ctx, &proto.ConsensusRequest{
		NodeID:            nodeID,
		NodeConsensusAddr: nodeConsensusAddr,
	})
	if errTalk != nil {
		return errorskit.Wrap(errTalk, errGrpcTalkLeader)
	}

	return nil
}

func ConsensusRemove(nodeID string, leaderGrpcAddr string) error {
	conn, errConn := protoclient.NewConnection(leaderGrpcAddr)
	if errConn != nil {
		return errConn
	}
	defer conn.Cleanup()

	_, errTalk := conn.Client.ConsensusRemove(conn.Ctx, &proto.ConsensusRequest{
		NodeID: nodeID,
	})
	if errTalk != nil {
		return errorskit.Wrap(errTalk, errGrpcTalkLeader)
	}

	return nil
}

// RequestNodeReinstall is called to prevent a case where a node has been stuck way too long
// incrementing terms and will cause problems with the cluster.
func RequestNodeReinstall(nodeGrpcAddr string) error {
	conn, errConn := protoclient.NewConnection(nodeGrpcAddr)
	if errConn != nil {
		return errConn
	}
	defer conn.Cleanup()

	_, _ = conn.Client.ReinstallNode(conn.Ctx, &proto.Empty{}) // Ignore the error
	return nil
}
