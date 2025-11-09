import { t, Language } from '../../i18n/translations'

interface FooterSectionProps {
  language: Language
}

export default function FooterSection({ language }: FooterSectionProps) {
  return (
    <footer style={{ borderTop: '1px solid var(--panel-border)', background: 'var(--brand-dark-gray)' }}>
      <div className='max-w-[1200px] mx-auto px-6 py-10'>
        {/* Brand */}


        {/* Multi-link columns */}

        {/* Bottom note (kept subtle) */}
        <div
          className='pt-6 mt-8 text-center text-xs'
          style={{ color: 'var(--text-tertiary)', borderTop: '1px solid var(--panel-border)' }}
        >
          <p>{t('footerTitle', language)}</p>
          <p className='mt-1'>{t('footerWarning', language)}</p>
        </div>
      </div>
    </footer>
  )
}
