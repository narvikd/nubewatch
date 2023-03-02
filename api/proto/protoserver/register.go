package protoserver

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"nubewatch/api/proto"
	"nubewatch/cluster"
	"nubewatch/cluster/consensus/fsm"
	"nubewatch/register"
	"nubewatch/register/registermodel"
	"time"
)

func (srv *server) RegisterServer(ctx context.Context, req *proto.RegisterServerReq) (*proto.RegisterServerRes, error) {
	if req.ServiceName == "" || req.Host == "" || req.Protocol == "" || req.HealthCheckEndpoint == "" {
		return nil, errors.New("serviceName, host, protocol or healthCheckEndpoint was empty")
	}

	rSrv := registermodel.Server{
		ServiceName:         req.ServiceName,
		ID:                  uuid.New().String(),
		Host:                req.Host,
		Protocol:            req.Protocol,
		HealthCheckEndpoint: req.HealthCheckEndpoint,
		CleanFuncName:       req.CleanFuncName,
		LastContact:         time.Now(),
		Alive:               true,
	}

	p := &fsm.Payload{
		Key:       req.ServiceName,
		Value:     rSrv,
		Operation: "ADD_SERVICE",
	}

	newServerIDAny, errCluster := cluster.Execute(srv.Node.Consensus, p)
	if errCluster != nil {
		return nil, errCluster
	}
	rSrv.ID = newServerIDAny.(string)

	go register.CheckServer(srv.Node.Consensus, rSrv)

	return &proto.RegisterServerRes{Id: rSrv.ID}, nil
}

func (srv *server) GetServers(ctx context.Context, req *proto.GetServersReq) (*proto.GetServersRes, error) {
	var servers []*proto.RegisterServerModel
	if req.Id == "" {
		return nil, errors.New("id was empty")
	}

	result, err := srv.Node.FSM.GetServers(req.Id)
	if err != nil {
		return nil, err
	}

	for _, v := range result {
		servers = append(servers, &proto.RegisterServerModel{
			Id:            v.ID,
			Host:          v.Host,
			CleanFuncName: v.CleanFuncName,
			LastContact:   v.LastContact.Format(time.RFC3339),
			Alive:         v.Alive,
		})
	}

	return &proto.GetServersRes{Servers: servers}, nil
}
