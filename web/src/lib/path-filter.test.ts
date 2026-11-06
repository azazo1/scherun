import assert from 'node:assert/strict'
import { test } from 'node:test'
import { deriveFilter, normalizePath } from './path-filter.ts'

test('normalizePath 去掉末尾分隔符', () => {
  assert.equal(normalizePath('/a/b/'), '/a/b')
  assert.equal(normalizePath('/a/b///'), '/a/b')
  assert.equal(normalizePath('/a/b'), '/a/b')
  // 根目录保持单个分隔符.
  assert.equal(normalizePath('/'), '/')
})

test('输入为空时不过滤', () => {
  assert.deepEqual(deriveFilter(''), { filter: '', isHidden: false })
  assert.deepEqual(deriveFilter('   '), { filter: '', isHidden: false })
})

test('与当前目录一致时不过滤', () => {
  // 每次加载成功都会把输入框同步成当前目录, 此时不能把列表过滤没.
  assert.deepEqual(deriveFilter('/Users/me/projects', '/Users/me/projects'), {
    filter: '',
    isHidden: false,
  })
  // 末尾分隔符不同也算同一目录.
  assert.deepEqual(deriveFilter('/Users/me/projects/', '/Users/me/projects'), {
    filter: '',
    isHidden: false,
  })
})

test('输入子路径前缀时按最后一段过滤', () => {
  assert.deepEqual(deriveFilter('/Users/me/pro', '/Users/me'), {
    filter: 'pro',
    isHidden: false,
  })
})

test('最后一段以点开头时标记为隐藏查询', () => {
  const result = deriveFilter('/Users/me/.loc', '/Users/me')
  assert.equal(result.filter, '.loc')
  assert.equal(result.isHidden, true)
})

test('单独输入点也触发隐藏查询', () => {
  const result = deriveFilter('.', '/Users/me')
  assert.equal(result.filter, '.')
  assert.equal(result.isHidden, true)
})

test('路径中间的隐藏段不影响隐藏判定', () => {
  // /Users/me/.local/bin 的最后一段是 bin, 不算隐藏查询.
  const result = deriveFilter('/Users/me/.local/bin', '/Users/me')
  assert.equal(result.filter, 'bin')
  assert.equal(result.isHidden, false)
})

test('反斜杠分隔同样适用', () => {
  const result = deriveFilter('C:\\Users\\me\\.git', 'C:\\Users\\me')
  assert.equal(result.filter, '.git')
  assert.equal(result.isHidden, true)
})

test('输入 /Users/azazo1/. 时只过滤点, 且触发隐藏查询', () => {
  // 用户敲下末尾的点时, 期望看到该目录下的隐藏条目,
  // 而不是把整个输入替换成当前目录.
  const result = deriveFilter('/Users/azazo1/.', '/Users/azazo1')
  assert.equal(result.filter, '.')
  assert.equal(result.isHidden, true)
})

test('以分隔符结尾时最后一段为空, 不误触发隐藏', () => {
  const result = deriveFilter('/Users/azazo1/', '/Users/azazo1')
  assert.equal(result.filter, '')
  assert.equal(result.isHidden, false)
})
