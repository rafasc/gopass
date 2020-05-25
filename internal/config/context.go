package config

import (
	"context"

	"github.com/gopasspw/gopass/internal/store/leaf"
	"github.com/gopasspw/gopass/pkg/ctxutil"
)

// WithContext returns a context with all config options set for this store
// config, iff they have not been already set in the context
func (c StoreConfig) WithContext(ctx context.Context) context.Context {
	if !ctxutil.HasAskForMore(ctx) {
		ctx = ctxutil.WithAskForMore(ctx, c.AskForMore)
	}
	if !ctxutil.HasAutoClip(ctx) {
		ctx = ctxutil.WithAutoClip(ctx, c.AutoClip)
	}
	if !c.AutoImport {
		ctx = leaf.WithImportFunc(ctx, nil)
	}
	if !leaf.HasAutoSync(ctx) {
		ctx = leaf.WithAutoSync(ctx, c.AutoSync)
	}
	if !ctxutil.HasClipTimeout(ctx) {
		ctx = ctxutil.WithClipTimeout(ctx, c.ClipTimeout)
	}
	if !ctxutil.HasConcurrency(ctx) {
		ctx = ctxutil.WithConcurrency(ctx, c.Concurrency)
	}
	if !ctxutil.HasEditRecipients(ctx) {
		ctx = ctxutil.WithEditRecipients(ctx, c.EditRecipients)
	}
	if !leaf.HasExportKeys(ctx) {
		ctx = leaf.WithExportKeys(ctx, c.ExportKeys)
	}
	if !ctxutil.HasNoConfirm(ctx) {
		ctx = ctxutil.WithNoConfirm(ctx, c.NoConfirm)
	}
	if !ctxutil.HasNoPager(ctx) {
		ctx = ctxutil.WithNoPager(ctx, c.NoPager)
	}
	if !ctxutil.HasNotifications(ctx) {
		ctx = ctxutil.WithNotifications(ctx, c.Notifications)
	}
	if !ctxutil.HasShowSafeContent(ctx) {
		ctx = ctxutil.WithShowSafeContent(ctx, c.SafeContent)
	}
	if !ctxutil.HasUseSymbols(ctx) {
		ctx = ctxutil.WithUseSymbols(ctx, c.UseSymbols)
	}
	// always disable autoclip when redirecting stdout
	if !ctxutil.IsTerminal(ctx) {
		ctx = ctxutil.WithAutoClip(ctx, false)
	}
	return ctx
}
