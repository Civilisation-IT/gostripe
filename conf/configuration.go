package conf

import (
	"os"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"github.com/sirupsen/logrus"
)

// DBConfiguration holds all the database related configuration.
type DBConfiguration struct {
	Driver         string `json:"driver" default:"postgres"`
	URL            string `json:"url" envconfig:"DATABASE_URL" required:"true"`
	Namespace      string `json:"namespace"`
	MigrationsPath string `json:"migrations_path" split_words:"true" default:"./migrations"`
}

// StripeConfiguration holds all the Stripe related configuration.
type StripeConfiguration struct {
	SecretKey      string `json:"secret_key" envconfig:"STRIPE_SECRET_KEY" required:"true"`
	PublishableKey string `json:"publishable_key" envconfig:"STRIPE_PUBLISHABLE_KEY" required:"true"`
	WebhookSecret  string `json:"webhook_secret" envconfig:"STRIPE_WEBHOOK_SECRET" required:"true"`
}

// JWTConfiguration holds the JWT related configuration.
type JWTConfiguration struct {
	Secret string `json:"secret" envconfig:"JWT_SECRET" required:"true"`
	Exp    int    `json:"exp" envconfig:"JWT_EXP" default:"3600"` // 1 hour
	Aud    string `json:"aud" envconfig:"JWT_AUD" default:"obex"`
}

// LoggingConfig holds the logging related configuration.
type LoggingConfig struct {
	Level string `json:"level" envconfig:"LOG_LEVEL" default:"info"`
	File  string `json:"file" envconfig:"LOG_FILE"`
}

// GlobalConfiguration holds all the configuration that applies to all instances.
type GlobalConfiguration struct {
	API struct {
		Host            string `envconfig:"API_HOST"`
		Port            int    `envconfig:"PORT" default:"8082"`
		Endpoint        string
		RequestIDHeader string `envconfig:"REQUEST_ID_HEADER"`
	}
	DB              DBConfiguration
	Stripe          StripeConfiguration
	JWT             JWTConfiguration
	Logging         LoggingConfig `envconfig:"LOG"`
	OperatorToken   string        `envconfig:"OPERATOR_TOKEN" required:"true"`
	RateLimitHeader string        `split_words:"true"`
}

// LoadGlobal loads configuration from file and environment variables.
func LoadGlobal(filename string) (*GlobalConfiguration, error) {
	if err := loadEnvironment(filename); err != nil {
		return nil, err
	}

	config := new(GlobalConfiguration)
	if err := envconfig.Process("", config); err != nil {
		return nil, err
	}

	if _, err := ConfigureLogging(&config.Logging); err != nil {
		return nil, err
	}

	return config, nil
}

func loadEnvironment(filename string) error {
	var err error
	if filename != "" {
		err = godotenv.Load(filename)
	} else {
		err = godotenv.Load()
		// It's ok if .env doesn't exist
		if os.IsNotExist(err) {
			return nil
		}
	}
	return err
}

// ConfigureLogging configures the logrus logger based on the configuration.
func ConfigureLogging(config *LoggingConfig) (*logrus.Logger, error) {
	logger := logrus.StandardLogger()

	if config.File != "" {
		f, err := os.OpenFile(config.File, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return nil, err
		}
		logger.SetOutput(f)
	}

	level, err := logrus.ParseLevel(config.Level)
	if err != nil {
		return nil, err
	}
	logger.SetLevel(level)

	return logger, nil
}
