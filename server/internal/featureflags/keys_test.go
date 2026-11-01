package featureflags

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/pkg/featureflag"
)

func TestChannelFlagsDefaultOff(t *testing.T) {
	ctx := context.Background()
	flags := featureflag.NewService(nil)
	for _, tc := range []struct {
		channel string
		enabled func(context.Context, *featureflag.Service) bool
	}{
		{"slack", ChannelSlackEnabled},
		{"telegram", ChannelTelegramEnabled},
		{"dingtalk", ChannelDingtalkEnabled},
		{"wecom", ChannelWecomEnabled},
	} {
		t.Run(tc.channel, func(t *testing.T) {
			if tc.enabled(ctx, flags) {
				t.Errorf("%s channel is enabled without any FF_CHANNEL_* override; Feishu must be the only default channel", tc.channel)
			}
		})
	}
}

func TestChannelFlagsReadEnvOverrides(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		channel string
		env     string
		enabled func(context.Context, *featureflag.Service) bool
	}{
		{"slack", "FF_CHANNEL_SLACK", ChannelSlackEnabled},
		{"telegram", "FF_CHANNEL_TELEGRAM", ChannelTelegramEnabled},
		{"dingtalk", "FF_CHANNEL_DINGTALK", ChannelDingtalkEnabled},
		{"wecom", "FF_CHANNEL_WECOM", ChannelWecomEnabled},
	} {
		t.Run(tc.channel, func(t *testing.T) {
			// One channel on must not turn any of the other three on.
			t.Setenv(tc.env, "true")
			flags, err := featureflag.NewServiceFromEnv()
			if err != nil {
				t.Fatalf("build feature flag service: %v", err)
			}
			if !tc.enabled(ctx, flags) {
				t.Errorf("%s channel stays off with %s=true", tc.channel, tc.env)
			}
			for _, other := range []struct {
				channel string
				enabled func(context.Context, *featureflag.Service) bool
			}{
				{"slack", ChannelSlackEnabled},
				{"telegram", ChannelTelegramEnabled},
				{"dingtalk", ChannelDingtalkEnabled},
				{"wecom", ChannelWecomEnabled},
			} {
				if other.channel == tc.channel {
					continue
				}
				if other.enabled(ctx, flags) {
					t.Errorf("enabling %s also enabled %s; each channel degrades on its own switch", tc.env, other.channel)
				}
			}
		})
	}
}

func TestEvaluateFrontendPublicFlagsPublishesChannelFlags(t *testing.T) {
	ctx := context.Background()
	channelKeys := []string{ChannelSlack, ChannelTelegram, ChannelDingtalk, ChannelWecom}

	flags, err := featureflag.NewServiceFromEnv()
	if err != nil {
		t.Fatalf("build feature flag service: %v", err)
	}
	for _, key := range channelKeys {
		enabled, ok := EvaluateFrontendPublicFlags(ctx, flags)[key]
		if !ok {
			t.Errorf("%q is not published to the frontend; the settings page and agent Integrations tab read it to hide the channel", key)
			continue
		}
		if enabled {
			t.Errorf("%q is published as enabled by default", key)
		}
	}

	t.Setenv("FF_CHANNEL_SLACK", "true")
	flags, err = featureflag.NewServiceFromEnv()
	if err != nil {
		t.Fatalf("build feature flag service: %v", err)
	}
	if enabled := EvaluateFrontendPublicFlags(ctx, flags)[ChannelSlack]; !enabled {
		t.Errorf("%q is not published as enabled with FF_CHANNEL_SLACK=true", ChannelSlack)
	}
}
