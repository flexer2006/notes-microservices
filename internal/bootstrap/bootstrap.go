package bootstrap

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	authv1 "github.com/flexer2006/notes-microservices/gen/auth/v1"
	notesv1 "github.com/flexer2006/notes-microservices/gen/notes/v1"
	"github.com/flexer2006/notes-microservices/internal/adapters/bcrypt"
	grpcadapter "github.com/flexer2006/notes-microservices/internal/adapters/grpc"
	httpServer "github.com/flexer2006/notes-microservices/internal/adapters/http"
	jwtadapter "github.com/flexer2006/notes-microservices/internal/adapters/jwt"
	postgresadapter "github.com/flexer2006/notes-microservices/internal/adapters/postgres"
	"github.com/flexer2006/notes-microservices/internal/adapters/redis"
	"github.com/flexer2006/notes-microservices/internal/app"
	"github.com/flexer2006/notes-microservices/internal/logger"
	"github.com/flexer2006/notes-microservices/internal/ports"
)

const (
	tokenCleanupInterval   = time.Hour
	defaultAccessTokenTTL  = 15 * time.Minute
	defaultRefreshTokenTTL = 24 * time.Hour
	httpBodyLimitBytes     = 1 << 20
)

func StartAuth(ctx context.Context, configPath string) error {
	cfg, err := initService(ctx, configPath)
	if err != nil {
		return err
	}

	database, err := setupPostgres(ctx, cfg, "auth")
	if err != nil {
		return err
	}

	jwtErr := ensureAuthJWT(cfg)
	if jwtErr != nil {
		database.Close(ctx)

		return jwtErr
	}

	serviceStartupLog(ctx, cfg, "authentication")

	repoFactory := postgresadapter.NewAuthRepositoryFactory(database.Pool())
	tokenRepo := repoFactory.TokenRepository()
	passwordSvc := bcrypt.NewBcrypt(cfg.JWT.BCryptCost)
	accessTTL := parseDurationOrDefault(cfg.JWT.AccessTokenTTL, defaultAccessTokenTTL)
	refreshTTL := parseDurationOrDefault(cfg.JWT.RefreshTokenTTL, defaultRefreshTokenTTL)
	tokenSvc := jwtadapter.NewIssuer(
		cfg.JWT.ResolvedAccessSecret(),
		cfg.JWT.ResolvedRefreshSecret(),
		accessTTL,
		refreshTTL,
	)
	authUC := app.NewAuthUseCase(repoFactory.UserRepository(), tokenRepo, passwordSvc, tokenSvc)
	userUC := app.NewUserUseCase(repoFactory.UserRepository())
	authHandler := grpcadapter.NewAuthHandler(authUC)
	userHandler := grpcadapter.NewUserHandler(userUC)
	grpcServer := grpcadapter.New(cfg, grpc.ChainUnaryInterceptor(
		grpcadapter.NewUnaryAuthInterceptor(
			tokenSvc,
			authv1.UserService_GetUserProfile_FullMethodName,
		),
	))
	grpcServer.RegisterService(authHandler.RegisterService)
	grpcServer.RegisterService(userHandler.RegisterService)

	startErr := grpcServer.Start(ctx)
	if startErr != nil {
		database.Close(ctx)

		return startErr
	}

	cleanupCtx, stopCleanup := context.WithCancel(context.WithoutCancel(ctx))

	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)

		runTokenCleanup(cleanupCtx, tokenRepo, tokenCleanupInterval)
	}()

	return Wait(ctx, time.Duration(cfg.Shutdown.Timeout)*time.Second, grpcServer.Err(),
		func(ctx context.Context) error {
			grpcServer.Stop(ctx)

			return nil
		},
		func(_ context.Context) error {
			stopCleanup()
			<-cleanupDone

			return nil
		},
		func(ctx context.Context) error {
			database.Close(ctx)

			return nil
		},
	)
}

func runTokenCleanup(ctx context.Context, tokenRepo ports.TokenRepository, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := tokenRepo.CleanupExpiredTokens(ctx)
			if err != nil {
				logger.Log(ctx).Error(ctx, "refresh token cleanup failed", zap.Error(err))
			}
		}
	}
}

func StartNotes(ctx context.Context, configPath string) error {
	cfg, err := initService(ctx, configPath)
	if err != nil {
		return err
	}

	database, err := setupPostgres(ctx, cfg, "notes")
	if err != nil {
		return err
	}

	jwtErr := ensureNotesJWT(cfg)
	if jwtErr != nil {
		database.Close(ctx)

		return jwtErr
	}

	serviceStartupLog(ctx, cfg, "notes")

	repoFactory := postgresadapter.NewRepositoryFactory(database.Pool())
	tokenSvc := jwtadapter.NewAccessVerifier(cfg.JWT.ResolvedAccessSecret())
	noteUseCase := app.NewNoteUseCase(repoFactory.NoteRepository())
	noteHandler := grpcadapter.NewNoteHandler(noteUseCase)
	grpcServer := grpcadapter.New(cfg, grpc.ChainUnaryInterceptor(
		grpcadapter.NewUnaryAuthInterceptor(tokenSvc,
			notesv1.NoteService_CreateNote_FullMethodName,
			notesv1.NoteService_GetNote_FullMethodName,
			notesv1.NoteService_ListNotes_FullMethodName,
			notesv1.NoteService_UpdateNote_FullMethodName,
			notesv1.NoteService_DeleteNote_FullMethodName,
		),
	))
	grpcServer.RegisterService(noteHandler.RegisterService)

	startErr := grpcServer.Start(ctx)
	if startErr != nil {
		database.Close(ctx)

		return startErr
	}

	return Wait(ctx, time.Duration(cfg.Shutdown.Timeout)*time.Second, grpcServer.Err(),
		func(ctx context.Context) error {
			grpcServer.Stop(ctx)

			return nil
		},
		func(ctx context.Context) error {
			database.Close(ctx)

			return nil
		},
	)
}

func StartGateway(ctx context.Context, configPath string) error {
	cfg, err := initService(ctx, configPath)
	if err != nil {
		return err
	}

	cfgErr := requireGatewayConfig(cfg)
	if cfgErr != nil {
		return cfgErr
	}

	serviceStartupLog(ctx, cfg, "gateway")

	authClient, err := grpcadapter.NewAuthClient(ctx, cfg)
	if err != nil {
		return err
	}

	notesClient, err := grpcadapter.NewNotesClient(ctx, cfg)
	if err != nil {
		_ = authClient.Close()

		return err
	}

	redisCache, err := redis.NewRedisCache(ctx, cfg)
	if err != nil {
		_ = notesClient.Close()
		_ = authClient.Close()

		return err
	}

	authService := app.NewAuthService(authClient, redisCache)
	notesService := app.NewNotesService(notesClient, redisCache)

	fiberCfg := fiber.Config{
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		BodyLimit:    httpBodyLimitBytes,
		ProxyHeader:  fiber.HeaderXForwardedFor,
		TrustProxy:   cfg.HTTP.TrustProxy,
		TrustProxyConfig: fiber.TrustProxyConfig{
			Proxies:    nil,
			LinkLocal:  false,
			Loopback:   false,
			Private:    cfg.HTTP.TrustPrivateProxies,
			UnixSocket: false,
		},
	}
	fiberApp := fiber.New(fiberCfg)
	//nolint:contextcheck
	httpServer.SetupRouter(fiberApp, authService, notesService, redisCache)

	address := net.JoinHostPort(cfg.HTTP.Host, strconv.Itoa(cfg.HTTP.Port))

	listener, err := new(net.ListenConfig).Listen(ctx, "tcp", address)
	if err != nil {
		_ = redisCache.Close()
		_ = notesClient.Close()
		_ = authClient.Close()

		return fmt.Errorf("bind http server on %s: %w", address, err)
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- fiberApp.Listener(listener)
	}()

	return Wait(ctx, time.Duration(cfg.Shutdown.Timeout)*time.Second, serveErr,
		func(ctx context.Context) error { return fiberApp.ShutdownWithContext(ctx) },
		func(_ context.Context) error { return authClient.Close() },
		func(_ context.Context) error { return notesClient.Close() },
		func(_ context.Context) error { return redisCache.Close() },
	)
}
