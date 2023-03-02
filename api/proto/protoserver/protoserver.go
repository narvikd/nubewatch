// Package protoserver provides nubewatch's gRPC server implementation.
package protoserver

import (
	"google.golang.org/grpc"
	"log"
	"net"
	"nubewatch/api/proto"
	"nubewatch/cluster/consensus"
)

// server represents the gRPC server.
type server struct {
	proto.UnimplementedServiceServer
	Node *consensus.Node
}

// Start starts the gRPC server.
func Start(n *consensus.Node) {
	protoServer := grpc.NewServer()
	listen, errListen := net.Listen("tcp", n.GrpcAddress)
	if errListen != nil {
		log.Fatalln(errListen)
	}

	proto.RegisterServiceServer(protoServer, &server{Node: n}) // register the server model

	errServe := protoServer.Serve(listen)
	if errServe != nil {
		log.Fatalln(errServe)
	}
}
