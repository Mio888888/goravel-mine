package modulecatalog

type lifecycleFailureTransition struct {
	item            LifecycleResultItem
	retainLock      bool
	watchLateRunner bool
}

type lifecycleStateMachine struct{}

func (lifecycleStateMachine) failure(item LifecycleResultItem, err error) lifecycleFailureTransition {
	item.Status = lifecycleErrorStatus(err)
	item.Error = lifecycleErrText(err)
	lateRunner := lifecycleCommandMayStillBeRunning(err)
	return lifecycleFailureTransition{
		item:            item,
		retainLock:      lateRunner,
		watchLateRunner: lateRunner,
	}
}

func (lifecycleStateMachine) success(item LifecycleResultItem) LifecycleResultItem {
	item.Status = LifecycleStatusSucceeded
	item.Error = ""
	return item
}
