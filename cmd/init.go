package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Set up GCP project and service account for Google Sheets access",
	Long: `Interactively sets up a GCP project and service account for perfi to
use when accessing Google Sheets. Enables the Sheets API, grants your
account permission to impersonate the service account, and saves the
configuration to ~/.perfi.yaml.

You can bring your own project and/or service account with flags, or
let perfi create them for you.`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().String("project", "", "existing GCP project ID to use")
	initCmd.Flags().String("service-account", "", "existing service account email to use")

	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	if _, err := exec.LookPath("gcloud"); err != nil {
		return fmt.Errorf("gcloud CLI not found — install it from https://cloud.google.com/sdk/docs/install")
	}

	scanner := bufio.NewScanner(os.Stdin)
	out := cmd.OutOrStdout()

	// --- GCP Project ---
	project, _ := cmd.Flags().GetString("project")
	if project == "" {
		fmt.Fprint(out, "Enter an existing GCP project ID (or leave blank to create one): ")
		scanner.Scan()
		project = strings.TrimSpace(scanner.Text())
	}

	if project == "" {
		fmt.Fprint(out, "Enter a new project ID to create: ")
		scanner.Scan()
		project = strings.TrimSpace(scanner.Text())
		if project == "" {
			return fmt.Errorf("a GCP project ID is required")
		}

		fmt.Fprintf(out, "Creating project %s...\n", project)
		if _, err := runGcloud("projects", "create", project, "--name=perfi"); err != nil {
			return fmt.Errorf("creating project: %w", err)
		}
	}

	// --- Service Account ---
	saEmail, _ := cmd.Flags().GetString("service-account")
	if saEmail == "" {
		fmt.Fprint(out, "Enter an existing service account email (or leave blank to create one): ")
		scanner.Scan()
		saEmail = strings.TrimSpace(scanner.Text())
	}

	if saEmail != "" {
		// Validate the SA belongs to the specified project.
		saProject := extractProjectFromSAEmail(saEmail)
		if saProject == "" {
			return fmt.Errorf("could not parse project from service account email %q — expected format: name@project.iam.gserviceaccount.com", saEmail)
		}
		if saProject != project {
			return fmt.Errorf("service account project %q does not match GCP project %q — they must be the same", saProject, project)
		}
	} else {
		saName := "perfi-sheets"
		fmt.Fprintf(out, "Enter a service account name [%s]: ", saName)
		scanner.Scan()
		if input := strings.TrimSpace(scanner.Text()); input != "" {
			saName = input
		}

		saEmail = fmt.Sprintf("%s@%s.iam.gserviceaccount.com", saName, project)
		fmt.Fprintf(out, "Creating service account %s...\n", saEmail)
		if _, err := runGcloud(
			"iam", "service-accounts", "create", saName,
			"--display-name=perfi Sheets access",
			"--project="+project,
		); err != nil {
			return fmt.Errorf("creating service account: %w", err)
		}
	}

	// --- Enable Sheets API ---
	fmt.Fprintf(out, "Enabling Google Sheets API...\n")
	if _, err := runGcloud("services", "enable", "sheets.googleapis.com", "--project="+project); err != nil {
		return fmt.Errorf("enabling Sheets API: %w", err)
	}

	// --- Grant impersonation permission ---
	userEmail, err := runGcloud("config", "get-value", "account")
	if err != nil {
		return fmt.Errorf("getting current gcloud account: %w", err)
	}
	if userEmail == "" {
		return fmt.Errorf("no gcloud account configured — run 'gcloud auth login' first")
	}

	fmt.Fprintf(out, "Granting %s permission to impersonate %s...\n", userEmail, saEmail)
	if _, err := runGcloud(
		"iam", "service-accounts", "add-iam-policy-binding", saEmail,
		"--member=user:"+userEmail,
		"--role=roles/iam.serviceAccountTokenCreator",
		"--project="+project,
	); err != nil {
		return fmt.Errorf("granting impersonation permission: %w", err)
	}

	// --- Write config ---
	viper.Set("gcp_project", project)
	viper.Set("service_account", saEmail)

	if err := writeConfig(); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	// --- Print next steps ---
	fmt.Fprintf(out, "\nSetup complete!\n\n")
	fmt.Fprintf(out, "Next steps:\n")
	fmt.Fprintf(out, "  1. Share your Google Sheet with the service account as an Editor:\n")
	fmt.Fprintf(out, "     %s\n\n", saEmail)
	fmt.Fprintf(out, "  2. If you haven't already, authenticate for local development:\n")
	fmt.Fprintf(out, "     gcloud auth application-default login\n\n")
	fmt.Fprintf(out, "  3. Configure your sheet ID and asset ranges in ~/.perfi.yaml\n")

	return nil
}

// writeConfig persists the current viper state to the config file.
func writeConfig() error {
	if err := viper.WriteConfig(); err != nil {
		// Config file doesn't exist yet — create it.
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return homeErr
		}
		return viper.WriteConfigAs(home + "/.perfi.yaml")
	}
	return nil
}

// runGcloud executes a gcloud command and returns its stdout.
func runGcloud(args ...string) (string, error) {
	cmd := exec.Command("gcloud", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gcloud %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// extractProjectFromSAEmail extracts the project ID from a service account email
// of the form name@project.iam.gserviceaccount.com.
func extractProjectFromSAEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return ""
	}
	domain := parts[1]
	suffix := ".iam.gserviceaccount.com"
	if !strings.HasSuffix(domain, suffix) {
		return ""
	}
	return strings.TrimSuffix(domain, suffix)
}
