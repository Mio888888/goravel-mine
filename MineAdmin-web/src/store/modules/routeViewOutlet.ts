import { defineComponent, h } from 'vue'
import { RouterView } from 'vue-router'

export default defineComponent({
  name: 'DynamicRouteViewOutlet',
  setup: () => () => h(RouterView),
})
