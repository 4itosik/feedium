import { BrowserRouter, Routes, Route } from 'react-router-dom'
import Layout from './components/Layout'
import FeedView from './views/FeedView'
import PostsView from './views/PostsView'
import SourcesView from './views/SourcesView'
import InterestsView from './views/InterestsView'
import DigestsView from './views/DigestsView'

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Layout />}>
          <Route index element={<FeedView />} />
          <Route path="posts" element={<PostsView />} />
          <Route path="digests" element={<DigestsView />} />
          <Route path="sources" element={<SourcesView />} />
          <Route path="interests" element={<InterestsView />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
