package modules

type Catalog struct {
	Modules []ModuleCatalog
}

type ModuleCatalog struct {
	ID            string
	Enabled       bool
	Reason        string
	Metadata      Metadata
	Package       Package
	Routes        []Route
	Menus         []Menu
	Permissions   []Permission
	Migrations    []string
	OpenAPIFiles  []string
	TestTemplates []string
}

func NewCatalog(items []Module) Catalog {
	modules := make([]ModuleCatalog, 0, len(items))
	for _, item := range items {
		modules = append(modules, catalogModule(item, true))
	}

	return Catalog{Modules: modules}
}

func catalogModule(item Module, enabled bool) ModuleCatalog {
	migrations := make([]string, 0, len(item.Migrations()))
	for _, migration := range item.Migrations() {
		migrations = append(migrations, migration.Signature())
	}

	return ModuleCatalog{
		ID:            item.ID(),
		Enabled:       enabled,
		Metadata:      ModuleMetadata(item),
		Package:       modulePackage(item),
		Routes:        item.Routes(),
		Menus:         item.Menus(),
		Permissions:   item.Permissions(),
		Migrations:    migrations,
		OpenAPIFiles:  item.OpenAPIFiles(),
		TestTemplates: item.TestTemplates(),
	}
}
