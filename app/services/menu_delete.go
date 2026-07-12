package services

type menuDeleteRow struct {
	ID       uint64 `gorm:"column:id"`
	ParentID uint64 `gorm:"column:parent_id"`
	Name     string `gorm:"column:name"`
}

func collectMenuDeleteTargets(rows []menuDeleteRow, roots []uint64) ([]uint64, []string) {
	children := make(map[uint64][]menuDeleteRow)
	byID := make(map[uint64]menuDeleteRow, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
		children[row.ParentID] = append(children[row.ParentID], row)
	}

	seen := make(map[uint64]bool)
	menuIDs := make([]uint64, 0, len(rows))
	menuNames := make([]string, 0, len(rows))
	var visit func(id uint64)
	visit = func(id uint64) {
		if seen[id] {
			return
		}
		row, ok := byID[id]
		if !ok {
			return
		}
		seen[id] = true
		menuIDs = append(menuIDs, row.ID)
		menuNames = append(menuNames, row.Name)
		for _, child := range children[id] {
			visit(child.ID)
		}
	}

	for _, id := range roots {
		visit(id)
	}
	return menuIDs, menuNames
}
