package factory

import (
	"context"
	"fmt"
	"log/slog"

	"eric-odp-factory/internal/common"
)

func (f *OdpFactoryImpl) createOdpToken(ctx context.Context, odp *OnDemandPod) error {
	odpToken, err := f.tsc.CreateToken(ctx, odp.Username, odp.TokenTypes)
	if err != nil {
		wrappedErr := fmt.Errorf("failed to create token: %w", err)
		slog.Error("createOdpToken", common.CtxIDLabel, ctx.Value(common.CtxID), "err", wrappedErr.Error())
		setOdpError(ctx, odp, wrappedErr, TokenError)

		return wrappedErr
	}

	odp.TokenName = odpToken.Name
	odp.TokenData = odpToken.Data

	return nil
}

func (f *OdpFactoryImpl) deleteOdpToken(ctx context.Context, odp *OnDemandPod) {
	if err := f.tsc.DeleteToken(ctx, odp.TokenName); err != nil {
		slog.Error("Failed to delete ODP Token", common.CtxIDLabel, ctx.Value(common.CtxID),
			"TokenName", odp.TokenName, "err", err)
	}
}

func (f *OdpFactoryImpl) readOdpToken(ctx context.Context, odp *OnDemandPod) error {
	odpToken, err := f.tsc.GetToken(ctx, odp.TokenName)
	if err != nil {
		return err //nolint:wrapcheck // wrapping with in done in caller
	}
	odp.TokenData = odpToken.Data

	return nil
}
