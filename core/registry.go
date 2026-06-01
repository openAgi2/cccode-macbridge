package core

import "fmt"

// AgentFactory creates an Agent from config options.
type AgentFactory func(opts map[string]any) (Agent, error)

var (
	agentFactories = make(map[string]AgentFactory)
)

func RegisterAgent(name string, factory AgentFactory) {
	agentFactories[name] = factory
}

func ListRegisteredAgents() []string {
	names := make([]string, 0, len(agentFactories))
	for k := range agentFactories {
		names = append(names, k)
	}
	return names
}

func CreateAgent(name string, opts map[string]any) (Agent, error) {
	f, ok := agentFactories[name]
	if !ok {
		available := make([]string, 0, len(agentFactories))
		for k := range agentFactories {
			available = append(available, k)
		}
		return nil, fmt.Errorf("unknown agent %q, available: %v", name, available)
	}
	return f(opts)
}
