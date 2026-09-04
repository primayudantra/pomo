package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var taskTag string

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage tasks",
}

var taskAddCmd = &cobra.Command{
	Use:   "add [name]",
	Short: "Add a task",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(c *cobra.Command, args []string) error {
		name := strings.Join(args, " ")
		id, err := database.AddTask(name, taskTag)
		if err != nil {
			return err
		}
		fmt.Printf("Added task #%d: %s\n", id, name)
		return nil
	},
}

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List open tasks",
	RunE: func(c *cobra.Command, args []string) error {
		tasks, err := database.ListOpenTasks()
		if err != nil {
			return err
		}
		if len(tasks) == 0 {
			fmt.Println("No open tasks. Add one with `pomo task add \"...\"`.")
			return nil
		}
		fmt.Println("TODAY")
		fmt.Println()
		fmt.Printf("%-4s %s\n", "ID", "Task")
		for _, t := range tasks {
			fmt.Printf("%-4d %s\n", t.ID, t.Name)
		}
		return nil
	},
}

var taskDoneCmd = &cobra.Command{
	Use:   "done [id]",
	Short: "Mark a task as done",
	Args:  cobra.ExactArgs(1),
	RunE: func(c *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task id: %s", args[0])
		}
		if err := database.CompleteTask(id); err != nil {
			return err
		}
		fmt.Printf("Task #%d marked done.\n", id)
		return nil
	},
}

var taskDeleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(c *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task id: %s", args[0])
		}
		if err := database.DeleteTask(id); err != nil {
			return err
		}
		fmt.Printf("Task #%d deleted.\n", id)
		return nil
	},
}

func init() {
	taskAddCmd.Flags().StringVar(&taskTag, "tag", "", "optional tag")
	taskCmd.AddCommand(taskAddCmd, taskListCmd, taskDoneCmd, taskDeleteCmd)
	rootCmd.AddCommand(taskCmd)
}
