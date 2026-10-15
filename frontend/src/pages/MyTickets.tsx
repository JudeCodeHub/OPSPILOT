import { useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { motion } from 'motion/react'
import { Link } from 'react-router'
import { api } from '../api'
import { ErrorState } from '../components/ErrorState'
import { Spinner } from '../components/Spinner'
import { TicketCard } from '../components/TicketCard'
import { lastEmail } from '../lib/storage'

export function MyTickets() {
  const [input, setInput] = useState(lastEmail.get)
  // The email actually searched for; starts with the remembered one.
  const [email, setEmail] = useState(() => lastEmail.get().trim().toLowerCase())

  const tickets = useQuery({
    queryKey: ['tickets', email],
    queryFn: () => api.listTickets(email),
    enabled: email !== '',
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const value = input.trim().toLowerCase()
    if (!value) return
    lastEmail.set(value)
    setEmail(value)
  }

  return (
    <div className="mx-auto max-w-3xl px-4 pt-10 sm:px-6 sm:pt-16">
      <h1 className="text-4xl font-bold sm:text-5xl">My tickets</h1>
      <p className="mt-3 text-ink-soft">Enter the email you booked with to see your tickets.</p>

      <form onSubmit={submit} className="glass mt-8 flex flex-col gap-3 rounded-3xl p-4 sm:flex-row sm:p-5">
        <label htmlFor="ticket-email" className="sr-only">
          Email
        </label>
        <input
          id="ticket-email"
          type="email"
          required
          autoComplete="email"
          className="input flex-1"
          placeholder="you@example.com"
          value={input}
          onChange={(e) => setInput(e.target.value)}
        />
        <button type="submit" className="btn-primary sm:px-8" disabled={tickets.isFetching && email === input.trim().toLowerCase()}>
          {tickets.isFetching ? <Spinner label="Finding…" /> : 'Find tickets'}
        </button>
      </form>

      <section className="mt-10" aria-live="polite" aria-label="Tickets">
        {tickets.isPending && email !== '' && (
          <div className="space-y-6" aria-busy="true">
            <div className="skeleton h-44 rounded-3xl" />
            <div className="skeleton h-44 rounded-3xl" />
          </div>
        )}

        {tickets.isError && <ErrorState error={tickets.error} onRetry={() => tickets.refetch()} retrying={tickets.isFetching} />}

        {tickets.isSuccess && tickets.data.length === 0 && (
          <div className="glass rounded-3xl p-10 text-center">
            <p className="font-display text-lg font-semibold">No tickets for {email} yet</p>
            <p className="mt-1 text-sm text-ink-soft">Tickets appear here a few seconds after checkout.</p>
            <Link to="/" className="btn-primary mt-6">
              Browse events
            </Link>
          </div>
        )}

        {tickets.isSuccess && tickets.data.length > 0 && (
          <motion.ul className="space-y-6" initial="hidden" animate="show" variants={{ show: { transition: { staggerChildren: 0.08 } } }}>
            {tickets.data.map((t) => (
              <motion.li key={t.code} variants={{ hidden: { opacity: 0, y: 16 }, show: { opacity: 1, y: 0 } }}>
                <TicketCard ticket={t} />
              </motion.li>
            ))}
          </motion.ul>
        )}
      </section>
    </div>
  )
}
