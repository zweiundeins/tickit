package demo

// PageSeederOf is the registered page seeder, for a test that replaces it.
func PageSeederOf() PageSeeder { return registeredPageSeeder() }
