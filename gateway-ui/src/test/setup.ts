import { config } from '@vue/test-utils'
import { defineComponent } from 'vue'

const commonProps = [
  'value',
  'modelValue',
  'checked',
  'open',
  'spinning',
  'options',
  'title',
  'subTitle',
  'message',
  'description',
  'dataSource',
  'columns',
  'locale',
  'buttonStyle',
  'size',
  'column',
  'gutter',
  'align',
  'bordered',
  'disabled',
  'placeholder',
  'loading',
  'block',
  'allowClear',
  'min',
  'max',
  'step',
  'style',
  'class',
  'autoSize',
  'copyable',
  'mode',
  'pagination',
  'rowKey',
  'xs',
  'sm',
  'xl',
  'span',
  'color',
  'label',
  'extra',
  'orientation',
  'type',
  'suffix',
  'ghost',
  'danger',
  'destroyOnClose',
  'width'
]

const passThroughStub = (tag = 'div') =>
  defineComponent({
    props: commonProps,
    emits: ['update:value', 'update:checked', 'update:open', 'change', 'click'],
    template: `<${tag}><slot /></${tag}>`
  })

const pageHeaderStub = defineComponent({
  props: ['title', 'subTitle'],
  template: `
    <header>
      <h1>{{ title }}</h1>
      <p v-if="subTitle">{{ subTitle }}</p>
      <slot />
      <slot name="extra" />
    </header>
  `
})

const cardStub = defineComponent({
  props: ['title'],
  template: `<section><h2 v-if="title">{{ title }}</h2><slot /></section>`
})

const alertStub = defineComponent({
  props: ['message', 'description', 'type'],
  template: `
    <div>
      <span v-if="message">{{ message }}</span>
      <span v-else-if="description">{{ description }}</span>
      <slot />
      <slot name="description" />
    </div>
  `
})

const formItemStub = defineComponent({
  props: ['label', 'extra'],
  template: `
    <div>
      <label v-if="label">{{ label }}</label>
      <small v-if="extra">{{ extra }}</small>
      <slot />
    </div>
  `
})

const statisticStub = defineComponent({
  props: ['title', 'value', 'suffix'],
  template: '<div class="stat">{{ title }}:{{ value }}{{ suffix || "" }}<slot /></div>'
})

const descriptionsItemStub = defineComponent({
  props: ['label'],
  template: '<div><strong v-if="label">{{ label }}</strong><slot /></div>'
})

const listStub = defineComponent({
  props: ['dataSource', 'locale'],
  template: '<div><slot /></div>'
})

const selectStub = defineComponent({
  props: ['value', 'modelValue', 'options', 'mode'],
  emits: ['update:value', 'change'],
  template: `
    <div>
      <span v-if="value !== undefined">{{ value }}</span>
      <span v-else-if="modelValue !== undefined">{{ modelValue }}</span>
      <span v-for="(option, index) in options || []" :key="index">
        {{ typeof option === 'object' ? option.label : option }}
      </span>
      <slot />
    </div>
  `
})

const inputStub = defineComponent({
  props: ['value', 'modelValue', 'disabled', 'placeholder'],
  emits: ['update:value', 'change'],
  template: '<div><span>{{ value ?? modelValue }}</span><slot /></div>'
})

const switchStub = defineComponent({
  props: ['checked'],
  emits: ['update:checked', 'change'],
  template: '<div>{{ checked ? "true" : "false" }}</div>'
})

const buttonStub = defineComponent({
  props: ['type', 'loading', 'ghost', 'block', 'danger'],
  emits: ['click'],
  template: '<button @click="$emit(\'click\')"><slot /></button>'
})

config.global.renderStubDefaultSlot = true
config.global.stubs = {
  'router-link': passThroughStub('a'),
  'router-view': passThroughStub(),
  'a-space': passThroughStub(),
  'a-row': passThroughStub(),
  'a-col': passThroughStub(),
  'a-card': cardStub,
  'a-statistic': statisticStub,
  'a-tag': passThroughStub('span'),
  'a-layout': passThroughStub(),
  'a-layout-sider': passThroughStub(),
  'a-layout-header': passThroughStub('header'),
  'a-layout-content': passThroughStub('main'),
  'a-menu': passThroughStub('nav'),
  'a-app': passThroughStub(),
  'a-typography-title': passThroughStub('h1'),
  'a-typography-text': passThroughStub('span'),
  'a-typography-paragraph': passThroughStub('p'),
  'a-page-header': pageHeaderStub,
  'a-alert': alertStub,
  'a-spin': passThroughStub(),
  'a-empty': passThroughStub(),
  'a-form': passThroughStub('form'),
  'a-form-item': formItemStub,
  'a-input': inputStub,
  'a-input-password': inputStub,
  'a-input-number': inputStub,
  'a-select': selectStub,
  'a-select-option': passThroughStub('span'),
  'a-switch': switchStub,
  'a-divider': passThroughStub('hr'),
  'a-table': passThroughStub('table'),
  'a-drawer': passThroughStub('aside'),
  'a-textarea': inputStub,
  'a-descriptions': passThroughStub('dl'),
  'a-descriptions-item': descriptionsItemStub,
  'a-button': buttonStub,
  'a-tabs': passThroughStub(),
  'a-tab-pane': passThroughStub(),
  'a-steps': passThroughStub(),
  'a-step': passThroughStub(),
  'a-list': listStub,
  'a-list-item': passThroughStub('div'),
  'a-tooltip': passThroughStub('span'),
  'a-radio-group': passThroughStub(),
  'a-radio-button': passThroughStub('button'),
  'a-checkbox': switchStub,
  'a-range-picker': passThroughStub(),
  'a-breadcrumb': passThroughStub('nav'),
  'a-breadcrumb-item': passThroughStub('span')
}

const aliasStubs = {
  ASelect: selectStub,
  AInput: inputStub,
  AInputPassword: inputStub,
  AInputNumber: inputStub,
  ATextarea: inputStub,
  ASwitch: switchStub,
  AButton: buttonStub,
  APageHeader: pageHeaderStub,
  AAlert: alertStub,
  ACard: cardStub,
  AFormItem: formItemStub,
  AStatistic: statisticStub
}
Object.assign(config.global.stubs, aliasStubs)

Object.defineProperty(globalThis, 'navigator', {
  configurable: true,
  value: {
    ...(globalThis.navigator ?? {}),
    clipboard: {
      writeText: async () => undefined
    }
  }
})

if (!globalThis.URL.createObjectURL) {
  globalThis.URL.createObjectURL = () => 'blob:mock-url'
}
if (!globalThis.URL.revokeObjectURL) {
  globalThis.URL.revokeObjectURL = () => undefined
}
