import { useCallback, useEffect, useRef, useState } from 'react'
import { api, ApiError } from '../lib/api'
import type { User } from '../types/api'

export function useAuth() {
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const generation = useRef(0)
  const controller = useRef<AbortController | null>(null)

  const onUnauthorized = useCallback(() => {
    generation.current++
    controller.current?.abort()
    setUser(null)
    setLoading(false)
  }, [])

  useEffect(() => {
    const current = ++generation.current
    const pending = new AbortController()
    controller.current = pending
    api.me(pending.signal).then((identity) => {
      if (generation.current === current && !pending.signal.aborted) setUser(identity)
    }).catch((cause: unknown) => {
      if (generation.current === current && !(cause instanceof ApiError && cause.status === 401)) {
        setError('Could not check your session.')
      }
    }).finally(() => {
      if (generation.current === current) setLoading(false)
    })
    return () => { generation.current++; pending.abort() }
  }, [])

  const login = useCallback(async (username: string, password: string) => {
    const current = ++generation.current
    controller.current?.abort()
    const pending = new AbortController()
    controller.current = pending
    setLoading(true)
    setError(null)
    try {
      const identity = await api.login(username, password, pending.signal)
      if (generation.current === current && !pending.signal.aborted) setUser(identity)
    } catch (cause) {
      if (generation.current === current) setError(cause instanceof Error ? cause.message : 'Sign in failed.')
      throw cause
    } finally {
      if (generation.current === current) setLoading(false)
    }
  }, [])

  const logout = useCallback(async () => {
    generation.current++
    controller.current?.abort()
    setUser(null)
    setError(null)
    try { await api.logout() }
    catch (cause) { setError(cause instanceof Error ? cause.message : 'Sign out failed.') }
  }, [])

  return { user, loading, error, login, logout, onUnauthorized }
}
