package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	pb "github.com/guilledipa/praetor/proto/gen/master"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	// Praetor Roman Theme Colors
	imperialPurple = lipgloss.Color("#602f6b")
	gold           = lipgloss.Color("#ffd700")
	marbleWhite    = lipgloss.Color("#fefdfa")
	crimsonRed     = lipgloss.Color("#8c001a")

	appStyle = lipgloss.NewStyle().Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Foreground(gold).
			Background(imperialPurple).
			Bold(true).
			Padding(0, 1).
			MarginBottom(1)

	agentListStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(imperialPurple).
			Padding(1).
			Width(30)

	detailStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(gold).
			Padding(1).
			Width(64)

	statusOK  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff00")).SetString("✓")
	statusErr = lipgloss.NewStyle().Foreground(crimsonRed).SetString("✗")
)

type item struct {
	nodeID    string
	compliant bool
}

func (i item) Title() string       { return i.nodeID }
func (i item) Description() string { 
	if i.compliant {
		return "Status: Active"
	}
	return "Status: DRIFT DETECTED"
}
func (i item) FilterValue() string { return i.nodeID }

type model struct {
	client   pb.OperatorClient
	list     list.Model
	selected *pb.AgentStatusResponse
	loading  bool
	err      error
}

type agentsLoadedMsg []*pb.AgentSummary
type statusLoadedMsg *pb.AgentStatusResponse
type syncTriggeredMsg string
type errMsg error

func initialModel(client pb.OperatorClient) model {
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 28, 20)
	l.Title = "Praetorian Guard"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = titleStyle

	return model{
		client:  client,
		list:    l,
		loading: true,
	}
}

func fetchAgents(client pb.OperatorClient) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{})
		if err != nil {
			return errMsg(err)
		}
		return agentsLoadedMsg(resp.GetAgents())
	}
}

func fetchStatus(client pb.OperatorClient, nodeID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		resp, err := client.GetAgentStatus(ctx, &pb.AgentStatusRequest{NodeId: nodeID})
		if err != nil {
			return errMsg(err)
		}
		return statusLoadedMsg(resp)
	}
}

func triggerSync(client pb.OperatorClient, nodeID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		resp, err := client.TriggerSync(ctx, &pb.TriggerSyncRequest{NodeId: nodeID})
		if err != nil {
			return errMsg(err)
		}
		if !resp.GetSuccess() {
			return errMsg(fmt.Errorf(resp.GetMessage()))
		}
		return syncTriggeredMsg(resp.GetMessage())
	}
}

func (m model) Init() tea.Cmd {
	return fetchAgents(m.client)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}
		if msg.String() == "enter" {
			i, ok := m.list.SelectedItem().(item)
			if ok {
				return m, fetchStatus(m.client, i.nodeID)
			}
		}
		if msg.String() == "r" {
			return m, fetchAgents(m.client)
		}
		if msg.String() == "s" {
			i, ok := m.list.SelectedItem().(item)
			if ok {
				return m, triggerSync(m.client, i.nodeID)
			}
		}
	case syncTriggeredMsg:
		m.list.NewStatusMessage(string(msg))
	case agentsLoadedMsg:
		m.loading = false
		var items []list.Item
		for _, a := range msg {
			items = append(items, item{nodeID: a.NodeId, compliant: true})
		}
		cardCount := len(items)
		if cardCount == 0 {
			m.list.SetItems([]list.Item{})
		} else {
			m.list.SetItems(items)
		}
	case statusLoadedMsg:
		m.selected = msg
	case errMsg:
		m.err = msg
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(marbleWhite).
		Background(imperialPurple).
		Width(96).
		Align(lipgloss.Center).
		Padding(1).
		Render("⚜  PRAETOR OPERATOR DASHBOARD  ⚜")

	var body string
	if m.err != nil {
		body = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(crimsonRed).
			Foreground(crimsonRed).
			Padding(2).
			Width(94).
			Align(lipgloss.Center).
			Render(fmt.Sprintf("Failed to communicate with Praetor Master Broker!\n\n%v\n\nEnsure docker-compose is running and certificates are generated.\nPress 'q' to retreat.", m.err))
	} else if m.loading {
		body = lipgloss.NewStyle().
			Foreground(gold).
			Padding(2).
			Width(96).
			Align(lipgloss.Center).
			Render("Summoning the Legions... (Connecting to Master)")
	} else {
		left := agentListStyle.Render(m.list.View())

		rightContent := "Select a Centurion (Node) to view compliance details\nPress Enter to inspect. Press 'r' to refresh cohort. Press 's' to manually trigger Sync."
		if m.selected != nil {
			rightContent = lipgloss.NewStyle().Foreground(gold).Bold(true).Render("Node: " + m.selected.NodeId) + "\n\n"
			if m.selected.IsCompliant {
				rightContent += statusOK.String() + " Fully Compliant\n\n"
			} else {
				rightContent += statusErr.String() + " Drift Detected\n\n"
			}
			rightContent += "Resource Trace Logs:\n"
			for _, r := range m.selected.Resources {
				icon := statusOK.String()
				if !r.Compliant {
					icon = statusErr.String()
				}
				msgStr := r.Message
				if len(msgStr) > 40 {
					msgStr = msgStr[:37] + "..."
				}
				rightContent += fmt.Sprintf("  %s [%s]\t%s\t%s\n", icon, r.Type, r.Id, msgStr)
			}
		}
		
		right := detailStyle.Render(rightContent)
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}

	return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, body))
}

func main() {
	// Usually PRAETORCTL would mount certificates passed by operator arguments.
	// We bind statically to workspace certificates for local dev.
	certPath := os.Getenv("PRAETOR_CERT")
	if certPath == "" {
		certPath = "../master/certs/server.crt"
		if _, err := os.Stat("../../../master/certs/server.crt"); err == nil {
			certPath = "../../../master/certs/server.crt"
		} else if _, err := os.Stat("master/certs/server.crt"); err == nil {
			certPath = "master/certs/server.crt"
		}
	}

	keyPath := os.Getenv("PRAETOR_KEY")
	if keyPath == "" {
		keyPath = "../master/certs/server.key"
		if _, err := os.Stat("../../../master/certs/server.key"); err == nil {
			keyPath = "../../../master/certs/server.key"
		} else if _, err := os.Stat("master/certs/server.key"); err == nil {
			keyPath = "master/certs/server.key"
		}
	}

	caPath := os.Getenv("PRAETOR_CA")
	if caPath == "" {
		caPath = "../master/certs/ca.crt"
		if _, err := os.Stat("../../../master/certs/ca.crt"); err == nil {
			caPath = "../../../master/certs/ca.crt"
		} else if _, err := os.Stat("master/certs/ca.crt"); err == nil {
			caPath = "master/certs/ca.crt"
		}
	}

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		fmt.Printf("Failed to load mTLS certs: %v\n", err)
		os.Exit(1)
	}
	caCert, err := os.ReadFile(caPath)
	if err != nil {
		fmt.Printf("Failed to read CA: %v\n", err)
		os.Exit(1)
	}
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caCert)

	creds := credentials.NewTLS(&tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            caPool,
		InsecureSkipVerify: true, // Bypass strict SAN checks for demo
	})

	conn, err := grpc.Dial("localhost:50053", grpc.WithTransportCredentials(creds))
	if err != nil {
		fmt.Printf("Failed to connect to Master Operator API: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	client := pb.NewOperatorClient(conn)

	p := tea.NewProgram(initialModel(client), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error routing the UI: %v", err)
		os.Exit(1)
	}
}
