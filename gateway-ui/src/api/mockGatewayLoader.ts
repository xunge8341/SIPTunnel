type MockGatewayModule = typeof import('./mockGateway')
type AnyMockFn = (...args: any[]) => any

type MockGatewayFunctionKey = {
  [K in keyof MockGatewayModule]: MockGatewayModule[K] extends AnyMockFn ? K : never
}[keyof MockGatewayModule]

let mockGatewayModulePromise: Promise<MockGatewayModule> | null = null

const loadMockGatewayModule = () => {
  if (mockGatewayModulePromise == null) {
    mockGatewayModulePromise = import('./mockGateway')
  }
  return mockGatewayModulePromise
}

export const callMockGateway = async <K extends MockGatewayFunctionKey>(
  key: K,
  ...args: Parameters<MockGatewayModule[K]>
): Promise<Awaited<ReturnType<MockGatewayModule[K]>>> => {
  const module = await loadMockGatewayModule()
  const handler = module[key] as AnyMockFn
  return handler(...args) as Awaited<ReturnType<MockGatewayModule[K]>>
}
