package comment

import (
	"context"
	"log"
	"net"

	"github.com/NTHU-LSALAB/NTHU-Distributed-System/modules/comment/dao"
	"github.com/NTHU-LSALAB/NTHU-Distributed-System/modules/comment/pb"
	"github.com/NTHU-LSALAB/NTHU-Distributed-System/modules/comment/service"
	"github.com/NTHU-LSALAB/NTHU-Distributed-System/pkg/logkit"
	"github.com/NTHU-LSALAB/NTHU-Distributed-System/pkg/pgkit"
	"github.com/NTHU-LSALAB/NTHU-Distributed-System/pkg/runkit"
	flags "github.com/jessevdk/go-flags"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

func newAPICommand() *cobra.Command {
	return &cobra.Command{
		Use:   "api",
		Short: "starts Comment API server",
		RunE:  runAPI,
	}
}

type APIArgs struct {
	GRPCAddr              string `long:"grpc_addr" env:"GRPC_ADDR" default:":8083"`
	runkit.GracefulConfig `group:"graceful" namespace:"graceful" env-namespace:"GRACEFUL"`
	logkit.LoggerConfig   `group:"logger" namespace:"logger" env-namespace:"LOGGER"`
	pgkit.PGConfig        `group:"pg" namespace:"pg" env-namespace:"PG"`
}

func runAPI(_ *cobra.Command, _ []string) error {
	ctx := context.Background()

	var args APIArgs
	if _, err := flags.NewParser(&args, flags.Default).Parse(); err != nil {
		log.Fatal("failed to parse flag", err.Error())
	}

	logger := logkit.NewLogger(&args.LoggerConfig)
	defer func() {
		if err := logger.Sync(); err != nil {
			log.Fatal("failed to sync logger", err.Error())
		}
	}()

	ctx = logger.WithContext(ctx)

	pgClient := pgkit.NewPGClient(ctx, &args.PGConfig)
	defer func() {
		if err := pgClient.Close(); err != nil {
			logger.Fatal("failed to close pg client", zap.Error(err))
		}
	}()

	commentPGDAO := dao.NewCommentPGDAO(pgClient)

	svc := service.NewService(commentPGDAO)

	logger.Info("listen to gRPC addr", zap.String("grpc_addr", args.GRPCAddr))
	lis, err := net.Listen("tcp", args.GRPCAddr)
	if err != nil {
		logger.Fatal("failed to listen gRPC addr", zap.Error(err))
	}
	defer func() {
		if err := lis.Close(); err != nil {
			logger.Fatal("failed to close gRPC listener", zap.Error(err))
		}
	}()

	return runkit.GracefulRun(serveGRPC(lis, svc, logger), &args.GracefulConfig)
}

func serveGRPC(lis net.Listener, svc pb.CommentServer, logger *logkit.Logger) runkit.GracefulRunFunc {
	grpcServer := grpc.NewServer()
	pb.RegisterCommentServer(grpcServer, svc)

	return func(ctx context.Context) error {
		go func() {
			if err := grpcServer.Serve(lis); err != nil {
				logger.Error("failed to run gRPC server", zap.Error(err))
			}
		}()

		<-ctx.Done()

		grpcServer.GracefulStop()

		return nil
	}
}