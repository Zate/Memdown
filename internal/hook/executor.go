package hook

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/zate/ctx/internal/db"
)

// ExecuteCommands processes parsed ctx commands against the database.
func ExecuteCommands(d *db.DB, commands []CtxCommand) error {
	for _, cmd := range commands {
		if err := executeCommand(d, cmd); err != nil {
			fmt.Fprintf(os.Stderr, "ctx warning: failed to execute %s command: %v\n", cmd.Type, err)
		}
	}
	return nil
}

// ExecuteCommandsWithErrors processes parsed ctx commands and returns errors.
func ExecuteCommandsWithErrors(d *db.DB, commands []CtxCommand) []error {
	var errs []error
	for _, cmd := range commands {
		if err := executeCommand(d, cmd); err != nil {
			errs = append(errs, fmt.Errorf("%s command failed: %w", cmd.Type, err))
		}
	}
	return errs
}

func executeCommand(d *db.DB, cmd CtxCommand) error {
	switch cmd.Type {
	case "remember":
		return executeRemember(d, cmd)
	case "recall":
		return executeRecall(d, cmd)
	case "summarize":
		return executeSummarize(d, cmd)
	case "link":
		return executeLink(d, cmd)
	case "status":
		return executeStatus(d)
	case "task":
		return executeTask(d, cmd)
	case "expand":
		return executeExpand(d, cmd)
	case "supersede":
		return executeSupersede(d, cmd)
	default:
		return fmt.Errorf("unknown command type: %s", cmd.Type)
	}
}

func executeRemember(d *db.DB, cmd CtxCommand) error {
	nodeType := cmd.Attrs["type"]
	if nodeType == "" {
		return fmt.Errorf("remember: type attribute is required")
	}
	content := strings.TrimSpace(cmd.Content)
	if content == "" {
		return fmt.Errorf("remember: content is required")
	}

	var tags []string
	if tagStr, ok := cmd.Attrs["tags"]; ok && tagStr != "" {
		tags = strings.Split(tagStr, ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
	}

	// Auto-add current task tag if working tier
	currentTask, err := d.GetPending("current_task")
	if err == nil && currentTask != "" {
		hasWorking := false
		for _, t := range tags {
			if t == "tier:working" {
				hasWorking = true
				break
			}
		}
		if hasWorking {
			tags = append(tags, "task:"+currentTask)
		}
	}

	_, err = d.CreateNode(db.CreateNodeInput{
		Type:    nodeType,
		Content: content,
		Tags:    tags,
	})
	return err
}

func executeRecall(d *db.DB, cmd CtxCommand) error {
	queryStr := cmd.Attrs["query"]
	if queryStr == "" {
		return fmt.Errorf("recall: query attribute is required")
	}

	// Import query package dynamically to avoid circular deps
	// Instead, we'll use the db directly with a simple approach
	// Store query for later execution by prompt-submit hook
	return d.SetPending("recall_query", queryStr)
}

func executeSummarize(d *db.DB, cmd CtxCommand) error {
	nodesStr := cmd.Attrs["nodes"]
	if nodesStr == "" {
		return fmt.Errorf("summarize: nodes attribute is required")
	}
	content := strings.TrimSpace(cmd.Content)
	if content == "" {
		return fmt.Errorf("summarize: content is required")
	}

	nodeIDs := strings.Split(nodesStr, ",")
	for i := range nodeIDs {
		nodeIDs[i] = strings.TrimSpace(nodeIDs[i])
	}

	archive := cmd.Attrs["archive"] == "true"

	summary, err := d.CreateNode(db.CreateNodeInput{
		Type:    "summary",
		Content: content,
	})
	if err != nil {
		return err
	}

	for _, sourceID := range nodeIDs {
		if _, err := d.CreateEdge(summary.ID, sourceID, "DERIVED_FROM"); err != nil {
			return fmt.Errorf("summarize: failed to create edge: %w", err)
		}
		if archive {
			_ = d.RemoveTag(sourceID, "tier:working")
			_ = d.RemoveTag(sourceID, "tier:reference")
			_ = d.RemoveTag(sourceID, "tier:pinned")
			_ = d.AddTag(sourceID, "tier:off-context")
		}
	}

	return nil
}

func executeLink(d *db.DB, cmd CtxCommand) error {
	fromID := cmd.Attrs["from"]
	toID := cmd.Attrs["to"]
	edgeType := cmd.Attrs["type"]
	if fromID == "" || toID == "" {
		return fmt.Errorf("link: from and to attributes are required")
	}
	if edgeType == "" {
		edgeType = "RELATES_TO"
	}

	_, err := d.CreateEdge(fromID, toID, edgeType)
	return err
}

func executeStatus(d *db.DB) error {
	// Generate status text and store in pending
	var totalNodes, totalTokens, edgeCount, tagCount int
	_ = d.QueryRow("SELECT COUNT(*) FROM nodes WHERE superseded_by IS NULL").Scan(&totalNodes)
	_ = d.QueryRow("SELECT COALESCE(SUM(token_estimate), 0) FROM nodes WHERE superseded_by IS NULL").Scan(&totalTokens)
	_ = d.QueryRow("SELECT COUNT(*) FROM edges").Scan(&edgeCount)
	_ = d.QueryRow("SELECT COUNT(DISTINCT tag) FROM tags").Scan(&tagCount)

	status := fmt.Sprintf("Nodes: %d (%d tokens), Edges: %d, Tags: %d unique", totalNodes, totalTokens, edgeCount, tagCount)

	// Add type breakdown
	rows, err := d.Query("SELECT type, COUNT(*) FROM nodes WHERE superseded_by IS NULL GROUP BY type ORDER BY type")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var t string
			var c int
			_ = rows.Scan(&t, &c)
			status += fmt.Sprintf("\n  %s: %d", t, c)
		}
	}

	return d.SetPending("status_output", status)
}

func executeTask(d *db.DB, cmd CtxCommand) error {
	name := cmd.Attrs["name"]
	action := cmd.Attrs["action"]

	if name == "" || action == "" {
		return fmt.Errorf("task: name and action attributes are required")
	}

	switch action {
	case "start":
		return d.SetPending("current_task", name)

	case "end":
		// Archive working nodes for this task
		rows, err := d.Query(`SELECT DISTINCT t1.node_id FROM tags t1
			JOIN tags t2 ON t1.node_id = t2.node_id
			WHERE t1.tag = ? AND t2.tag = 'tier:working'`, "task:"+name)
		if err != nil {
			return err
		}
		defer rows.Close()

		var nodeIDs []string
		for rows.Next() {
			var id string
			_ = rows.Scan(&id)
			nodeIDs = append(nodeIDs, id)
		}

		for _, id := range nodeIDs {
			// Check if it's a decision (keep in reference)
			node, err := d.GetNode(id)
			if err != nil {
				continue
			}
			if node.Type == "decision" {
				// Promote to reference
				_ = d.RemoveTag(id, "tier:working")
				_ = d.AddTag(id, "tier:reference")
			} else {
				// Archive
				_ = d.RemoveTag(id, "tier:working")
				_ = d.AddTag(id, "tier:off-context")
			}
		}

		_ = d.DeletePending("current_task")
		return nil

	default:
		return fmt.Errorf("task: unknown action %s", action)
	}
}

func executeExpand(d *db.DB, cmd CtxCommand) error {
	nodeID := cmd.Attrs["node"]
	if nodeID == "" {
		return fmt.Errorf("expand: node attribute is required")
	}

	// Get DERIVED_FROM edges
	edges, err := d.GetEdgesFrom(nodeID)
	if err != nil {
		return err
	}

	var sourceIDs []string
	for _, e := range edges {
		if e.Type == "DERIVED_FROM" {
			sourceIDs = append(sourceIDs, e.ToID)
		}
	}

	if len(sourceIDs) == 0 {
		return nil
	}

	data, _ := json.Marshal(sourceIDs)
	return d.SetPending("expand_nodes", string(data))
}

func executeSupersede(d *db.DB, cmd CtxCommand) error {
	oldID := cmd.Attrs["old"]
	newID := cmd.Attrs["new"]
	if oldID == "" || newID == "" {
		return fmt.Errorf("supersede: old and new attributes are required")
	}

	// Mark old as superseded
	_, err := d.Exec("UPDATE nodes SET superseded_by = ? WHERE id = ?", newID, oldID)
	if err != nil {
		return err
	}

	// Create SUPERSEDES edge
	_, err = d.CreateEdge(newID, oldID, "SUPERSEDES")
	return err
}
