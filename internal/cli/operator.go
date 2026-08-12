package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	app "github.com/fd1az/nats-tui/internal/app/operator"
	"github.com/fd1az/nats-tui/internal/config"
	domain "github.com/fd1az/nats-tui/internal/domain/authority"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type OperatorReader interface {
	ListOperatorCatalog(context.Context, config.Settings) (app.Catalog, error)
}

type OperatorAccountCreator interface {
	CreateOperatorAccount(context.Context, config.Settings, string, string, domain.AccountDefinition) (app.AccountArtifacts, error)
}

type OperatorUserCreator interface {
	CreateOperatorUser(context.Context, string, string, domain.UserDefinition) (app.UserArtifacts, error)
}

type OperatorMutator interface {
	UpdateOperatorAccount(context.Context, config.Settings, string, string, string, domain.AccountDefinition) (app.AccountArtifacts, error)
	DeleteOperatorAccount(context.Context, config.Settings, string, string, string) (string, error)
	UpdateOperatorUser(context.Context, string, string, domain.UserDefinition) (app.UserArtifacts, error)
	RevokeOperatorUser(context.Context, config.Settings, string, string, string, string) (app.AccountArtifacts, error)
	DeleteOperatorUser(context.Context, config.Settings, string, string, string, string) (string, error)
	AddOperatorSigningKey(context.Context, config.Settings, string, string, string, domain.SigningKeyDefinition) (app.SigningKeyArtifacts, error)
}

type operatorOutput struct {
	Capabilities app.Capabilities `json:"capabilities"`
	Accounts     []app.Account    `json:"accounts"`
	Users        []app.User       `json:"managed_users,omitempty"`
}

func newAccountsCommand(values *viper.Viper, reader OperatorReader, accountCreator OperatorAccountCreator, userCreator OperatorUserCreator, mutator OperatorMutator) *cobra.Command {
	var jsonOutput bool
	command := &cobra.Command{
		Use:     "accounts",
		Aliases: []string{"operator"},
		Short:   "Inspect NATS operator accounts and claims",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if reader == nil {
				return errors.New("operator catalog reader is not configured")
			}
			catalog, err := reader.ListOperatorCatalog(cmd.Context(), config.Load(values))
			if err != nil {
				return err
			}
			result := operatorOutput{Capabilities: catalog.Capabilities(), Accounts: catalog.Accounts(), Users: catalog.Users()}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			return writeAccounts(cmd.OutOrStdout(), result)
		},
	}
	command.Flags().BoolVar(&jsonOutput, "json", false, "print machine-readable JSON")
	command.AddCommand(newCreateAccountCommand(values, accountCreator))
	command.AddCommand(newUpdateAccountCommand(values, mutator))
	command.AddCommand(newDeleteAccountCommand(values, mutator))
	command.AddCommand(newSigningKeyCommand(values, mutator))
	command.AddCommand(newUserCommand(values, userCreator, mutator))
	return command
}

func newCreateAccountCommand(values *viper.Viper, creator OperatorAccountCreator) *cobra.Command {
	var operatorSeed, outputDirectory, description string
	limits := domain.AccountLimits{Connections: -1, LeafNodes: -1, Subscriptions: -1, Payload: -1, Data: -1, Exports: -1, Imports: -1, Streams: -1, Consumers: -1, Memory: -1, Storage: -1}
	command := &cobra.Command{Use: "create NAME", Short: "Create, securely persist, and publish an operator account", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if creator == nil {
			return errors.New("operator account creator is not configured")
		}
		settings := config.Load(values)
		if operatorSeed == "" {
			operatorSeed = settings.OperatorSeed
		}
		if outputDirectory == "" {
			outputDirectory = settings.AuthorityDir
		}
		if operatorSeed == "" || outputDirectory == "" {
			return errors.New("--operator-seed and --output-dir are required")
		}
		definition, err := domain.NewAccountDefinition(args[0], limits)
		if err != nil {
			return err
		}
		definition = definition.WithDescription(description)
		artifacts, err := creator.CreateOperatorAccount(cmd.Context(), settings, operatorSeed, outputDirectory, definition)
		if err != nil {
			if artifacts.SeedPath != "" {
				return fmt.Errorf("claim was not published; private artifacts remain at %s: %w", artifacts.SeedPath, err)
			}
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "created account %s\nseed: %s\nclaim: %s\n", args[0], artifacts.SeedPath, artifacts.JWTPath)
		return err
	}}
	flags := command.Flags()
	flags.StringVar(&operatorSeed, "operator-seed", "", "operator seed file used only for signing")
	flags.StringVar(&outputDirectory, "output-dir", "", "private output directory (files are created with mode 0600)")
	flags.StringVar(&description, "description", "", "account description")
	flags.Int64Var(&limits.Connections, "max-connections", -1, "maximum connections (-1 unlimited)")
	flags.Int64Var(&limits.Subscriptions, "max-subscriptions", -1, "maximum subscriptions (-1 unlimited)")
	flags.Int64Var(&limits.Streams, "max-streams", -1, "maximum streams (-1 unlimited)")
	flags.Int64Var(&limits.Consumers, "max-consumers", -1, "maximum consumers (-1 unlimited)")
	flags.Int64Var(&limits.Memory, "max-memory", -1, "maximum JetStream memory bytes (-1 unlimited)")
	flags.Int64Var(&limits.Storage, "max-storage", -1, "maximum JetStream disk bytes (-1 unlimited)")
	return command
}

func newUserCommand(values *viper.Viper, creator OperatorUserCreator, mutator OperatorMutator) *cobra.Command {
	command := &cobra.Command{Use: "user", Short: "Manage locally-authorized users"}
	var account, accountSeed, outputDirectory string
	var publishAllow, publishDeny, subscribeAllow, subscribeDeny []string
	var responseMax int
	var responseTTL time.Duration
	var subscriptionLimit int64
	create := &cobra.Command{Use: "create NAME", Short: "Create a user JWT and private credentials file", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if creator == nil {
			return errors.New("operator user creator is not configured")
		}
		if outputDirectory == "" {
			outputDirectory = config.Load(values).AuthorityDir
		}
		if account == "" || accountSeed == "" || outputDirectory == "" {
			return errors.New("--account, --account-seed, and --output-dir are required")
		}
		definition, err := domain.NewUserDefinition(args[0], account, domain.Permissions{PublishAllow: publishAllow, PublishDeny: publishDeny, SubscribeAllow: subscribeAllow, SubscribeDeny: subscribeDeny, ResponseMax: responseMax, ResponseTTL: responseTTL, SubscriptionLimit: subscriptionLimit})
		if err != nil {
			return err
		}
		artifacts, err := creator.CreateOperatorUser(cmd.Context(), accountSeed, outputDirectory, definition)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "created user %s\ncredentials: %s\n", args[0], artifacts.CredentialsPath)
		return err
	}}
	flags := create.Flags()
	flags.StringVar(&account, "account", "", "account public key")
	flags.StringVar(&accountSeed, "account-seed", "", "account seed file used only for signing")
	flags.StringVar(&outputDirectory, "output-dir", "", "private output directory (credentials use mode 0600)")
	flags.StringSliceVar(&publishAllow, "pub-allow", nil, "publish subjects to allow")
	flags.StringSliceVar(&publishDeny, "pub-deny", nil, "publish subjects to deny")
	flags.StringSliceVar(&subscribeAllow, "sub-allow", nil, "subscribe subjects to allow")
	flags.StringSliceVar(&subscribeDeny, "sub-deny", nil, "subscribe subjects to deny")
	flags.IntVar(&responseMax, "response-max", 0, "maximum response messages")
	flags.DurationVar(&responseTTL, "response-ttl", 0, "response permission TTL")
	flags.Int64Var(&subscriptionLimit, "max-subscriptions", -1, "maximum subscriptions (-1 unlimited)")
	command.AddCommand(create)
	command.AddCommand(newUpdateUserCommand(values, mutator))
	command.AddCommand(newRevokeUserCommand(values, mutator, false))
	command.AddCommand(newRevokeUserCommand(values, mutator, true))
	return command
}

func newUpdateAccountCommand(values *viper.Viper, mutator OperatorMutator) *cobra.Command {
	var seed, directory, description string
	limits := unlimitedAccountLimits()
	command := &cobra.Command{Use: "update ACCOUNT NAME", Aliases: []string{"edit"}, Short: "Update a locally managed account claim", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if mutator == nil {
			return errors.New("operator account updater is not configured")
		}
		settings := config.Load(values)
		seed, directory = authorityPaths(settings, seed, directory)
		if seed == "" || directory == "" {
			return errors.New("--operator-seed and --output-dir are required")
		}
		definition, err := domain.NewAccountDefinition(args[1], limits)
		if err != nil {
			return err
		}
		artifacts, err := mutator.UpdateOperatorAccount(cmd.Context(), settings, seed, directory, args[0], definition.WithDescription(description))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "updated account %s\nclaim: %s\n", args[0], artifacts.JWTPath)
		return err
	}}
	flags := command.Flags()
	flags.StringVar(&seed, "operator-seed", "", "operator seed file used only for signing")
	flags.StringVar(&directory, "output-dir", "", "private authority directory")
	flags.StringVar(&description, "description", "", "account description")
	addAccountLimitFlags(flags, &limits)
	return command
}

func newDeleteAccountCommand(values *viper.Viper, mutator OperatorMutator) *cobra.Command {
	var seed, directory string
	var yes bool
	command := &cobra.Command{Use: "delete ACCOUNT", Short: "Delete an account claim and archive local authority files", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if mutator == nil {
			return errors.New("operator account deleter is not configured")
		}
		if !yes {
			return errors.New("account deletion requires --yes")
		}
		settings := config.Load(values)
		seed, directory = authorityPaths(settings, seed, directory)
		if seed == "" || directory == "" {
			return errors.New("--operator-seed and --output-dir are required")
		}
		trash, err := mutator.DeleteOperatorAccount(cmd.Context(), settings, seed, directory, args[0])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "deleted account %s\nrecoverable files: %s\n", args[0], trash)
		return err
	}}
	command.Flags().StringVar(&seed, "operator-seed", "", "operator seed file used only for signing")
	command.Flags().StringVar(&directory, "output-dir", "", "private authority directory")
	command.Flags().BoolVar(&yes, "yes", false, "confirm destructive operation")
	return command
}

func newSigningKeyCommand(values *viper.Viper, mutator OperatorMutator) *cobra.Command {
	root := &cobra.Command{Use: "signing-key", Short: "Manage account signing keys"}
	var seed, directory, role, description string
	var scoped bool
	var pubAllow, pubDeny, subAllow, subDeny []string
	var responseMax int
	var responseTTL time.Duration
	var subscriptions int64 = -1
	add := &cobra.Command{Use: "add ACCOUNT", Short: "Create a regular or scoped account signing key", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if mutator == nil {
			return errors.New("signing key creator is not configured")
		}
		settings := config.Load(values)
		seed, directory = authorityPaths(settings, seed, directory)
		if seed == "" || directory == "" {
			return errors.New("--operator-seed and --output-dir are required")
		}
		definition, err := domain.NewSigningKeyDefinition(role, description, scoped, domain.Permissions{PublishAllow: pubAllow, PublishDeny: pubDeny, SubscribeAllow: subAllow, SubscribeDeny: subDeny, ResponseMax: responseMax, ResponseTTL: responseTTL, SubscriptionLimit: subscriptions})
		if err != nil {
			return err
		}
		artifacts, err := mutator.AddOperatorSigningKey(cmd.Context(), settings, seed, directory, args[0], definition)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "created signing key for %s\nseed: %s\n", args[0], artifacts.SeedPath)
		return err
	}}
	flags := add.Flags()
	flags.StringVar(&seed, "operator-seed", "", "operator seed file used only for signing")
	flags.StringVar(&directory, "output-dir", "", "private authority directory")
	flags.BoolVar(&scoped, "scoped", false, "restrict users signed by this key to a permission template")
	flags.StringVar(&role, "role", "", "required role name for a scoped key")
	flags.StringVar(&description, "description", "", "signing key description")
	flags.StringSliceVar(&pubAllow, "pub-allow", nil, "publish subjects allowed by the scope")
	flags.StringSliceVar(&pubDeny, "pub-deny", nil, "publish subjects denied by the scope")
	flags.StringSliceVar(&subAllow, "sub-allow", nil, "subscribe subjects allowed by the scope")
	flags.StringSliceVar(&subDeny, "sub-deny", nil, "subscribe subjects denied by the scope")
	flags.IntVar(&responseMax, "response-max", 0, "maximum response messages")
	flags.DurationVar(&responseTTL, "response-ttl", 0, "response permission TTL")
	flags.Int64Var(&subscriptions, "max-subscriptions", -1, "maximum subscriptions")
	root.AddCommand(add)
	return root
}

func newUpdateUserCommand(values *viper.Viper, mutator OperatorMutator) *cobra.Command {
	var directory, account string
	var pubAllow, pubDeny, subAllow, subDeny []string
	var responseMax int
	var responseTTL time.Duration
	var subscriptions int64 = -1
	command := &cobra.Command{Use: "update USER NAME", Aliases: []string{"edit"}, Short: "Update a managed user without rotating its NKey", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if mutator == nil {
			return errors.New("operator user updater is not configured")
		}
		if directory == "" {
			directory = config.Load(values).AuthorityDir
		}
		if directory == "" || account == "" {
			return errors.New("--account and --output-dir are required")
		}
		definition, err := domain.NewUserDefinition(args[1], account, domain.Permissions{PublishAllow: pubAllow, PublishDeny: pubDeny, SubscribeAllow: subAllow, SubscribeDeny: subDeny, ResponseMax: responseMax, ResponseTTL: responseTTL, SubscriptionLimit: subscriptions})
		if err != nil {
			return err
		}
		artifacts, err := mutator.UpdateOperatorUser(cmd.Context(), directory, args[0], definition)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "updated user %s\ncredentials: %s\n", args[0], artifacts.CredentialsPath)
		return err
	}}
	flags := command.Flags()
	flags.StringVar(&directory, "output-dir", "", "private authority directory")
	flags.StringVar(&account, "account", "", "account public key")
	flags.StringSliceVar(&pubAllow, "pub-allow", nil, "publish subjects to allow")
	flags.StringSliceVar(&pubDeny, "pub-deny", nil, "publish subjects to deny")
	flags.StringSliceVar(&subAllow, "sub-allow", nil, "subscribe subjects to allow")
	flags.StringSliceVar(&subDeny, "sub-deny", nil, "subscribe subjects to deny")
	flags.IntVar(&responseMax, "response-max", 0, "maximum response messages")
	flags.DurationVar(&responseTTL, "response-ttl", 0, "response permission TTL")
	flags.Int64Var(&subscriptions, "max-subscriptions", -1, "maximum subscriptions")
	return command
}

func newRevokeUserCommand(values *viper.Viper, mutator OperatorMutator, remove bool) *cobra.Command {
	var seed, directory string
	var yes bool
	verb := "revoke"
	short := "Revoke a managed user in its account claim"
	if remove {
		verb, short = "delete", "Revoke a user and archive its local credentials"
	}
	command := &cobra.Command{Use: verb + " ACCOUNT USER", Short: short, Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if mutator == nil {
			return errors.New("operator user mutator is not configured")
		}
		if !yes {
			return fmt.Errorf("user %s requires --yes", verb)
		}
		settings := config.Load(values)
		seed, directory = authorityPaths(settings, seed, directory)
		if seed == "" || directory == "" {
			return errors.New("--operator-seed and --output-dir are required")
		}
		if remove {
			trash, err := mutator.DeleteOperatorUser(cmd.Context(), settings, seed, directory, args[0], args[1])
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "deleted user %s\nrecoverable credentials: %s\n", args[1], trash)
			return err
		}
		artifacts, err := mutator.RevokeOperatorUser(cmd.Context(), settings, seed, directory, args[0], args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "revoked user %s\nclaim: %s\n", args[1], artifacts.JWTPath)
		return err
	}}
	command.Flags().StringVar(&seed, "operator-seed", "", "operator seed file used only for signing")
	command.Flags().StringVar(&directory, "output-dir", "", "private authority directory")
	command.Flags().BoolVar(&yes, "yes", false, "confirm destructive operation")
	return command
}

func unlimitedAccountLimits() domain.AccountLimits {
	return domain.AccountLimits{Connections: -1, LeafNodes: -1, Subscriptions: -1, Payload: -1, Data: -1, Exports: -1, Imports: -1, Streams: -1, Consumers: -1, Memory: -1, Storage: -1}
}

func addAccountLimitFlags(flags *pflag.FlagSet, limits *domain.AccountLimits) {
	flags.Int64Var(&limits.Connections, "max-connections", -1, "maximum connections")
	flags.Int64Var(&limits.LeafNodes, "max-leaf-nodes", -1, "maximum leaf connections")
	flags.Int64Var(&limits.Subscriptions, "max-subscriptions", -1, "maximum subscriptions")
	flags.Int64Var(&limits.Payload, "max-payload", -1, "maximum message payload bytes")
	flags.Int64Var(&limits.Data, "max-data", -1, "maximum data bytes")
	flags.Int64Var(&limits.Exports, "max-exports", -1, "maximum exports")
	flags.Int64Var(&limits.Imports, "max-imports", -1, "maximum imports")
	flags.Int64Var(&limits.Streams, "max-streams", -1, "maximum JetStream streams")
	flags.Int64Var(&limits.Consumers, "max-consumers", -1, "maximum JetStream consumers")
	flags.Int64Var(&limits.Memory, "max-memory", -1, "maximum JetStream memory bytes")
	flags.Int64Var(&limits.Storage, "max-storage", -1, "maximum JetStream disk bytes")
}

func authorityPaths(settings config.Settings, seed, directory string) (string, string) {
	if seed == "" {
		seed = settings.OperatorSeed
	}
	if directory == "" {
		directory = settings.AuthorityDir
	}
	return seed, directory
}

func writeAccounts(output io.Writer, result operatorOutput) error {
	mode := "ACCOUNT SCOPED"
	if result.Capabilities.OperatorMode {
		mode = "OPERATOR MODE"
	}
	if _, err := fmt.Fprintf(output, "%s  system=%t  claims-read=%t  claims-write=%t  local-authority=%t\n\n", mode, result.Capabilities.SystemAccount, result.Capabilities.ClaimsRead, result.Capabilities.ClaimsWrite, result.Capabilities.LocalAuthority); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(output, "ACCOUNT\tID\tCONNS\tSUBS\tCONN LIMIT\tSUB LIMIT\tIMPORTS\tEXPORTS"); err != nil {
		return err
	}
	for _, account := range result.Accounts {
		if _, err := fmt.Fprintf(output, "%s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\n", account.Name, account.ID, account.Connections, account.Subscriptions, account.Limits.Connections, account.Limits.Subscriptions, len(account.Imports), len(account.Exports)); err != nil {
			return err
		}
	}
	if len(result.Users) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(output, "\nMANAGED USER\tPUBLIC KEY\tACCOUNT\tPUB ALLOW\tSUB ALLOW\tSTATUS"); err != nil {
		return err
	}
	for _, user := range result.Users {
		status := "active"
		if user.Revoked {
			status = "revoked"
		}
		if _, err := fmt.Fprintf(output, "%s\t%s\t%s\t%d\t%d\t%s\n", user.Name, user.PublicKey, user.Account, len(user.PublishAllow), len(user.SubscribeAllow), status); err != nil {
			return err
		}
	}
	return nil
}
