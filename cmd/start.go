package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"pomo/internal/gitinfo"
	"pomo/internal/model"
	"pomo/internal/pomoconfig"
	"pomo/internal/tui"
)

var (
	startDuration int
	startTag      string
)

var startCmd = &cobra.Command{
	Use:   "start [task]",
	Short: "Start a Pomodoro session",
	Args:  cobra.ArbitraryArgs,
	RunE:  runStart,
}

func init() {
	startCmd.Flags().IntVar(&startDuration, "duration", 0, "session duration in minutes")
	startCmd.Flags().StringVar(&startTag, "tag", "", "optional tag for this session")
	rootCmd.AddCommand(startCmd)
}

func runStart(c *cobra.Command, args []string) error {
	if existing, err := database.LastRunningSession(); err == nil && existing != nil {
		fmt.Printf("A session is already running: %q. Finish or stop it first (`pomo stop`).\n", existing.TaskName)
		return nil
	}

	// No task given: hand off to the interactive, arrow-key-navigable app
	// (task picker -> timer -> back to dashboard, all in one program).
	if len(args) == 0 {
		return tui.RunAppTaskSelect(database)
	}

	cfg := pomoconfig.Load(database)
	taskName := strings.Join(args, " ")

	duration := cfg.Focus
	if startDuration > 0 {
		duration = time.Duration(startDuration) * time.Minute
	}

	taskID, err := database.FindOrCreateTask(taskName, startTag)
	if err != nil {
		return err
	}

	cwd, _ := os.Getwd()
	repoPath, repoBranch := gitinfo.Describe(cwd)

	sessionID, err := database.CreateSession(model.Session{
		TaskID:          taskID,
		TaskName:        taskName,
		Tag:             startTag,
		PlannedDuration: int(duration.Seconds()),
		Status:          model.StatusRunning,
		StartedAt:       time.Now(),
		RepoPath:        repoPath,
		RepoBranch:      repoBranch,
	})
	if err != nil {
		return err
	}

	fmt.Printf("\n🍅 %s\n\n%s\n\n", taskName, fmtMMSS(int(duration.Seconds())))

	timerModel := tui.NewTimer(taskName, int(duration.Seconds()))
	p := tea.NewProgram(timerModel)
	finalModel, err := p.Run()
	if err != nil {
		_ = database.FinishSession(sessionID, model.StatusInterrupted, 0, "")
		return err
	}
	tm := finalModel.(tui.TimerModel)

	status := model.SessionStatus(tm.Result.Status)
	if err := database.FinishSession(sessionID, status, tm.Result.ActualSeconds, ""); err != nil {
		return err
	}

	switch status {
	case model.StatusCompleted:
		fmt.Printf("🍅 Pomodoro completed!\n\nTask:\n%s\n\nDuration:\n%d minutes\n\n", taskName, tm.Result.ActualSeconds/60)
		note := promptNote()
		if note != "" {
			_ = database.SetSessionNote(sessionID, note)
		}
		fmt.Println("\n✓ Saved to history")
	case model.StatusSkipped:
		fmt.Println("\n⏭ Session skipped.")
	case model.StatusCancelled:
		fmt.Println("\n✗ Session cancelled.")
	}
	return nil
}

func fmtMMSS(secs int) string {
	return fmt.Sprintf("%02d:%02d", secs/60, secs%60)
}

func promptNote() string {
	fmt.Print("Add a note? (optional)\n\n> ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}
