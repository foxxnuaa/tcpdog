package grpc

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync"

	pb "github.com/mehrdadrad/tcpdog/proto"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/mehrdadrad/tcpdog/config"
	"github.com/mehrdadrad/tcpdog/egress/helper"
)

// StartStructPB sends fields to a grpc server with structpb type.
func StartStructPB(ctx context.Context, tp config.Tracepoint, bufpool *sync.Pool, ch chan *bytes.Buffer) error {
	var (
		stream pb.TCPDog_TracepointSPBClient
		conn   *grpc.ClientConn
	)

	cfg := config.FromContext(ctx)
	logger := cfg.Logger()
	backoff := helper.NewBackoff(logger)

	gCfg, err := gRPCConfig(cfg.Egress[tp.Egress].Config)
	if err != nil {
		return err
	}

	opts, err := dialOpts(gCfg)
	if err != nil {
		return err
	}

	go func() {
		for {
			backoff.Next()

			conn, err = grpc.Dial(gCfg.Server, opts...)
			if err != nil {
				logger.Warn("grpc", zap.Error(err))
				continue
			}

			client := pb.NewTCPDogClient(conn)
			stream, err = client.TracepointSPB(ctx)
			if err != nil {
				logger.Warn("grpc", zap.Error(err))
				conn.Close()
				continue
			}

			logger.Info("grpc", zap.String("msg",
				fmt.Sprintf("%s has been connected to %s", tp.Egress, gCfg.Server)))

			err = structpb(ctx, stream, tp, bufpool, ch)
			if err != nil {
				logger.Warn("grpc", zap.Error(err))
				conn.Close()
				continue
			}

			break
		}

	}()

	return nil
}

func structpb(ctx context.Context, stream pb.TCPDog_TracepointSPBClient, tp config.Tracepoint, bufpool *sync.Pool, ch chan *bytes.Buffer) error {
	var (
		cfg = config.FromContext(ctx)
		spb = helper.NewStructPB(cfg.Fields[tp.Fields])
		buf *bytes.Buffer
		err error
	)

	for {
		select {
		case buf = <-ch:
			err = stream.Send(&pb.FieldsSPB{
				Fields: spb.Unmarshal(buf),
			})
			if err != nil {
				return err
			}

			bufpool.Put(buf)
		case <-ctx.Done():
			stream.CloseAndRecv()
			return nil
		}
	}
}

func protobuf(ctx context.Context, stream pb.TCPDog_TracepointClient, bufpool *sync.Pool, ch chan *bytes.Buffer) error {
	var (
		buf         *bytes.Buffer
		hostname, _ = os.Hostname()
	)

	for {
		select {
		case buf = <-ch:
			m := pb.Fields{}
			protojson.Unmarshal(buf.Bytes(), &m)
			m.Hostname = &hostname
			if err := stream.Send(&m); err != nil {
				return err
			}

			bufpool.Put(buf)
		case <-ctx.Done():
			stream.CloseAndRecv()
			return nil
		}
	}
}

// Start sends fields to a grpc server
func Start(ctx context.Context, tp config.Tracepoint, bufpool *sync.Pool, ch chan *bytes.Buffer) error {
	var (
		stream pb.TCPDog_TracepointClient
		conn   *grpc.ClientConn
	)

	cfg := config.FromContext(ctx)
	logger := cfg.Logger()
	backoff := helper.NewBackoff(logger)

	gCfg, err := gRPCConfig(cfg.Egress[tp.Egress].Config)
	if err != nil {
		return err
	}

	opts, err := dialOpts(gCfg)
	if err != nil {
		return err
	}

	go func() {
		for {
			backoff.Next()

			conn, err = grpc.Dial(gCfg.Server, opts...)
			if err != nil {
				logger.Warn("grpc", zap.Error(err))
				continue
			}

			client := pb.NewTCPDogClient(conn)
			stream, err = client.Tracepoint(ctx)
			if err != nil {
				logger.Warn("grpc", zap.Error(err))
				conn.Close()
				continue
			}

			logger.Info("grpc", zap.String("msg",
				fmt.Sprintf("%s has been connected to %s", tp.Egress, gCfg.Server)))

			err = protobuf(ctx, stream, bufpool, ch)
			if err != nil {
				logger.Warn("grpc", zap.Error(err))
				conn.Close()
				continue
			}

			break
		}

	}()

	return nil
}

func dialOpts(gCfg *grpcConf) ([]grpc.DialOption, error) {
	var opts []grpc.DialOption

	if gCfg.TLSConfig.Enable {
		creds, err := config.GetCreds(&gCfg.TLSConfig)
		if err != nil {
			return nil, err
		}

		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithInsecure())
	}

	return opts, nil
}
