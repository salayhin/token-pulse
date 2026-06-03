# Token Pulse Logos

Professional logo designs for Token Pulse dashboard.

## Logo Versions

### 1. **logo-final.svg** ⭐ (Recommended)
- **Best for**: Main branding, website header, documentation
- **Design**: Complete design with:
  - Dashboard bars (representing analytics/data)
  - Pulse wave (real-time monitoring)
  - Growth arrow (upward trend)
  - Gradient blue colors (#3b82f6 to #1e40af)
  - Circular frame with subtle background
- **Size**: 240×240px (scalable)
- **Usage**: Primary logo for all official use

### 2. **logo-v2.svg** (Professional)
- **Best for**: Alternative branding, wider use cases
- **Design**: Stylized with:
  - Detailed pulse wave with heartbeat pattern
  - Lightning bolt accent (energy/power)
  - Outer ring frame
  - Gradient effect
  - Token indicator circles
- **Size**: 200×200px (scalable)
- **Usage**: Secondary logo option

### 3. **logo-v3.svg** (Minimalist)
- **Best for**: Favicon, small icons, simplified usage
- **Design**: Clean minimalist style:
  - Stacked dashboard bars
  - Simple pulse wave
  - Growth indicator arrow
  - High contrast, easy to scale down
- **Size**: 200×200px (scalable)
- **Usage**: Favicon, button icons, small elements

### 4. **logo.svg** (Simple)
- **Best for**: Quick reference, basic branding
- **Design**: Simple and clean:
  - Pulse waves at different opacities
  - Central lightning bolt
  - Minimal accent circles
- **Size**: 200×200px (scalable)
- **Usage**: Basic reference

## Color Scheme

- **Primary Blue**: #3b82f6 (Bright blue)
- **Secondary Blue**: #60a5fa (Light blue)
- **Dark Blue**: #1e40af (Dark blue)
- **Border**: #e5e7eb (Light gray)
- **Background**: #f0f4ff (Very light blue)

## Usage Guidelines

### Website Header
```html
<img src="assets/logos/logo-final.svg" alt="Token Pulse" width="40" height="40">
```

### Social Media / Documentation
- Use `logo-final.svg` at 512×512px or larger
- Maintain 1:1 aspect ratio
- Keep padding around the logo

### Favicon
- Convert `logo-v3.svg` to ICO/PNG format
- Recommended size: 32×32px, 64×64px
- Test for clarity at small sizes

### Print / High-Resolution
- Use `logo-final.svg` (vector format is resolution-independent)
- Minimum size: 1 inch wide
- Maintain clear space around logo

## Design Inspiration

- **Pulse Wave**: Represents real-time monitoring and live updates (SSE streaming)
- **Dashboard Bars**: Symbolizes data analytics and visualization
- **Lightning Bolt**: Conveys speed, energy, and power
- **Growth Arrow**: Indicates upward trends and positive metrics
- **Blue Gradient**: Professional, trustworthy, tech-forward

## Customization

All logos are SVG files and can be easily customized:

1. **Colors**: Edit the hex values in the SVG
2. **Sizing**: SVG scales infinitely without quality loss
3. **Stroke Width**: Adjust stroke-width values for different styles
4. **Opacity**: Modify opacity values for transparency effects

Example color change:
```bash
# Change primary blue to another color
sed -i 's/#3b82f6/#your-color/g' logo-final.svg
```

## Export Options

Convert SVG to other formats:

```bash
# PNG (using ImageMagick)
convert -density 300 logo-final.svg -background none logo-final.png

# ICO Favicon
convert -density 300 logo-v3.svg -define icon:auto-resize=32,64 favicon.ico

# PDF
inkscape logo-final.svg -o logo-final.pdf
```

---

**Recommended**: Use `logo-final.svg` for all primary branding needs.
