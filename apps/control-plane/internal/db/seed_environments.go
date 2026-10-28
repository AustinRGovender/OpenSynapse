package db

// Names of the environments seeded for the bundled test APIs (user-testing/).
const (
	TestAPIsDockerEnv = "Test APIs (Docker)"
	TestAPIsLocalEnv  = "Test APIs (localhost)"
)

// testAPIEnvironments returns the variables the built-in plans reference.
// Inside Docker Compose the APIs are reachable by service name; when they
// run with `go run` on the host they listen on localhost.
func testAPIEnvironments() map[string]map[string]Variable {
	return map[string]map[string]Variable{
		TestAPIsDockerEnv: {
			"ECHO_API":      {Value: "http://echo-api:9001"},
			"ECOMMERCE_API": {Value: "http://mock-ecommerce:9002"},
			"SLOW_API":      {Value: "http://slow-api:9003"},
			"ERROR_API":     {Value: "http://error-api:9004"},
		},
		TestAPIsLocalEnv: {
			"ECHO_API":      {Value: "http://localhost:9001"},
			"ECOMMERCE_API": {Value: "http://localhost:9002"},
			"SLOW_API":      {Value: "http://localhost:9003"},
			"ERROR_API":     {Value: "http://localhost:9004"},
		},
	}
}

// SeedTestAPIEnvironments creates the two test-API environments if missing
// and returns their IDs keyed by name.
func (s *EnvironmentStore) SeedTestAPIEnvironments() (map[string]string, error) {
	ids := map[string]string{}
	for name, vars := range testAPIEnvironments() {
		var id string
		err := s.db.QueryRow(`SELECT id FROM environments WHERE name = ?`, name).Scan(&id)
		if err == nil {
			ids[name] = id
			continue
		}
		env, err := s.Create(name, vars)
		if err != nil {
			return nil, err
		}
		ids[name] = env.ID
	}
	return ids, nil
}

// AssignBuiltInEnvironment sets the default environment of built-in plans
// that do not have one yet.
func (s *PlanStore) AssignBuiltInEnvironment(envID string) error {
	_, err := s.db.Exec(`UPDATE plans SET default_environment_id = ? WHERE built_in = 1 AND default_environment_id IS NULL`, envID)
	return err
}
