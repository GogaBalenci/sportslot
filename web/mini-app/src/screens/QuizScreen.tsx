import { useState } from 'react'
import { Button, Typography } from '@maxhub/max-ui'
import { api, type QuizResult } from '../api'
import { haptic } from '../max'
import { useNav } from '../nav'

export function QuizScreen() {
  const nav = useNav()
  const questions = nav.catalog?.quiz ?? []
  const [step, setStep] = useState(0)
  const [answers, setAnswers] = useState<Record<string, string>>({})
  const [results, setResults] = useState<QuizResult[] | null>(null)
  const [error, setError] = useState('')

  async function answer(questionId: string, optionId: string) {
    haptic('tap')
    const next = { ...answers, [questionId]: optionId }
    setAnswers(next)
    if (step + 1 < questions.length) {
      setStep(step + 1)
      return
    }
    try {
      setResults((await api.recommend(next)).results)
      haptic('success')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не получилось подобрать')
    }
  }

  function restart() {
    setStep(0)
    setAnswers({})
    setResults(null)
  }

  if (results) {
    return (
      <div className="screen">
        <header className="screen__header">
          <Typography.Headline variant="medium">Вам может подойти</Typography.Headline>
          <div className="muted">Начните с пробного занятия — это ни к чему не обязывает.</div>
        </header>
        {results.map((r, i) => (
          <article key={r.sport} className={`card${i === 0 ? ' card--top' : ''}`}>
            <Typography.Title variant="small-strong">{i + 1}. {r.title}</Typography.Title>
            <div className="muted">{r.reason}</div>
            <Button variant={i === 0 ? 'primary' : 'secondary'} onClick={() => nav.tab({ name: 'search', sport: r.sport })}>
              Найти занятия
            </Button>
          </article>
        ))}
        <button className="text-button" onClick={restart}>Пройти заново</button>
      </div>
    )
  }

  const q = questions[step]
  if (!q) return <div className="screen"><div className="center muted">Загружаем вопросы…</div></div>
  return (
    <div className="screen">
      <header className="screen__header">
        <div className="muted">Вопрос {step + 1} из {questions.length}</div>
        <div className="progress"><i style={{ width: `${((step + 1) / questions.length) * 100}%` }} /></div>
        <Typography.Headline variant="medium">{q.title}</Typography.Headline>
      </header>
      <div className="stack">
        {q.options.map((o) => (
          <Button key={o.id} size="large" variant={answers[q.id] === o.id ? 'primary' : 'secondary'} stretched onClick={() => answer(q.id, o.id)}>
            {o.title}
          </Button>
        ))}
      </div>
      {error && <div className="error">{error}</div>}
      {step > 0 && <button className="text-button" onClick={() => setStep(step - 1)}>Назад</button>}
    </div>
  )
}
