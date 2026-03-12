import { config } from '@vue/test-utils'

const passThroughStub = (tag = 'div') => ({
  template: `<${tag}><slot /></${tag}>`
})

config.global.stubs = {
  'a-space': passThroughStub(),
  'a-row': passThroughStub(),
  'a-col': passThroughStub(),
  'a-card': passThroughStub('section'),
  'a-statistic': {
    props: ['title', 'value', 'suffix'],
    template: '<div class="stat">{{ title }}:{{ value }}{{ suffix || "" }}</div>'
  },
  'a-tag': passThroughStub('span'),
  'a-layout': passThroughStub(),
  'a-layout-sider': passThroughStub(),
  'a-layout-header': passThroughStub('header'),
  'a-layout-content': passThroughStub('main'),
  'a-menu': passThroughStub('nav'),
  'a-app': passThroughStub(),
  'a-typography-title': passThroughStub('h1'),
  'a-button': {
    emits: ['click'],
    template: '<button @click="$emit(\'click\')"><slot /></button>'
  }
}
