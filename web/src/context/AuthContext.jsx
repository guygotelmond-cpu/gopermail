import { createContext, useState, useCallback } from 'react'

const USER_KEY = 'gm_user'
const TOKEN_KEY = 'gm_token'

export const AuthContext = createContext(null)

export function AuthProvider({ children }) {
  const [username, setUsername] = useState(() => localStorage.getItem(USER_KEY))
  const [token, setToken] = useState(() => localStorage.getItem(TOKEN_KEY))

  const login = useCallback((name, tok) => {
    localStorage.setItem(USER_KEY, name)
    localStorage.setItem(TOKEN_KEY, tok)
    setUsername(name)
    setToken(tok)
  }, [])

  const logout = useCallback(() => {
    localStorage.removeItem(USER_KEY)
    localStorage.removeItem(TOKEN_KEY)
    setUsername(null)
    setToken(null)
  }, [])

  return (
    <AuthContext.Provider value={{ username, token, login, logout }}>
      {children}
    </AuthContext.Provider>
  )
}
