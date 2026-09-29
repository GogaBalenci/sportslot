import { useState } from 'react'
import { ArrowRight, Sparkles } from 'lucide-react'
import { api, type QuizResult } from '../api'
import { Btn } from '../components'
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
        <header className="hero">
          <span className="badge badge--orange"><Sparkles size={12} /> Подбор готов</span>
          <h1 className="h1">Тебе может подойти</h1>
          <p className="muted">Начни с пробного занятия — это ни к чему не обязывает.</p>
        </header>
        {results.map((r, i) => (
          <article key={r.sport} className={`result${i === 0 ? ' result--top' : ''}`}>
            <div className="result__rank">{i + 1}</div>
            <div className="result__body">
              <h3 className="card__title">{r.title}</h3>
              <p className="muted">{r.reason}</p>
              <Btn variant={i === 0 ? 'primary' : 'secondary'} icon={<ArrowRight size={18} />} onClick={() => nav.tab({ name: 'search', sport: r.sport })}>
                Найти занятия
              </Btn>
            </div>
          </article>
        ))}
        <Btn variant="ghost" block onClick={restart}>Пройти заново</Btn>
      </div>
    )
  }

  const q = questions[step]
  if (!q) return <div className="screen"><div className="loader">Загружаем вопросы…</div></div>
  return (
    <div className="screen">
      <header className="hero">
        <div className="quiz-step">Вопрос {step + 1} из {questions.length}</div>
        <div className="progress"><i style={{ width: `${((step + 1) / questions.length) * 100}%` }} /></div>
        <h1 className="h1">{q.title}</h1>
      </header>
      <div className="stack">
        {q.options.map((o) => (
          <button key={o.id} className={`option${answers[q.id] === o.id ? ' option--active' : ''}`} onClick={() => answer(q.id, o.id)}>
            <span>{o.title}</span>
            <ArrowRight size={18} />
          </button>
        ))}
      </div>
      {error && <div className="error">{error}</div>}
      {step > 0 && <Btn variant="ghost" block onClick={() => setStep(step - 1)}>Назад</Btn>}
    </div>
  )
}
