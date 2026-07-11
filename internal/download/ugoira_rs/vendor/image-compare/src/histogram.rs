use crate::prelude::*;

const BINS: u8 = u8::MAX;

fn correlation(first_hist: &Histogram, second_hist: &Histogram) -> Option<f64> {
    let first_mean = first_hist.mean();
    let second_mean = second_hist.mean();
    let numerator = (0..=BINS)
        .map(|i| {
            (first_hist.get_bin_content(i) - first_mean)
                * (second_hist.get_bin_content(i) - second_mean)
        })
        .sum::<f64>();
    let denominator = (first_hist.variance() * second_hist.variance()).sqrt();

    if denominator != 0. {
        Some(numerator / denominator)
    } else {
        None
    }
}

fn chi_square(first_hist: &Histogram, second_hist: &Histogram) -> Option<f64> {
    let score = (0..=BINS)
        .map(|i| {
            let num = (first_hist.get_bin_content(i) - second_hist.get_bin_content(i)).powi(2);
            let den = first_hist.get_bin_content(i);
            if num == 0. {
                0.
            } else {
                num / den
            }
        })
        .sum::<f64>();

    if score.is_nan() || score.is_infinite() {
        None
    } else {
        Some(score)
    }
}

fn intersection(first_hist: &Histogram, second_hist: &Histogram) -> f64 {
    (0..=BINS)
        .map(|i| {
            first_hist
                .get_bin_content(i)
                .min(second_hist.get_bin_content(i))
        })
        .sum::<f64>()
}

fn hellinger(first_hist: &Histogram, second_hist: &Histogram) -> Option<f64> {
    let bc = (0..=BINS)
        .map(|i| (first_hist.get_bin_content(i) * second_hist.get_bin_content(i)).sqrt())
        .sum::<f64>();
    let normalization = (first_hist.integral() * second_hist.integral()).sqrt();
    if normalization == 0. {
        return None;
    }
    let bc_normalized = bc / normalization;
    Some((1. - bc_normalized).sqrt())
}

/// The distance metric choices for histogram comparisons
pub enum Metric {
    /// <img src="https://render.githubusercontent.com/render/math?math=d(H_1,H_2) = \frac{\sum_I (H_1(I) - \bar{H_1}) (H_2(I) - \bar{H_2})}{\sqrt{\sum_I(H_1(I) - \bar{H_1})^2 \sum_I(H_2(I) - \bar{H_2})^2}}">
    Correlation,
    /// <img src="https://render.githubusercontent.com/render/math?math=d(H_1,H_2) = \sum _I \frac{\left(H_1(I)-H_2(I)\right)^2}{H_1(I)}">
    /// First histogram may not have empty bins
    ChiSquare,
    /// <img src="https://render.githubusercontent.com/render/math?math=d(H_1,H_2) = \sum _I \min (H_1(I), H_2(I))">
    Intersection,
    /// <img src="https://render.githubusercontent.com/render/math?math=d(H_1,H_2) = \sqrt{1 - \frac{1}{\sqrt{\int{H_1} \int{H_2}}} \sum_I \sqrt{H_1(I) \cdot H_2(I)}}">
    /// Both histograms need to be normalizable
    Hellinger,
}

pub fn img_compare(
    first: &GrayImage,
    second: &GrayImage,
    metric: Metric,
) -> Result<f64, CompareError> {
    let first_hist = Histogram::from_gray_image(first);
    let second_hist = Histogram::from_gray_image(second);
    let score = match metric {
        Metric::Correlation => correlation(&first_hist, &second_hist).ok_or_else(|| {
            CompareError::CalculationFailed(
                "One or both histograms' variances were zero!".to_owned(),
            )
        })?,
        Metric::ChiSquare => chi_square(&first_hist, &second_hist).ok_or_else(|| {
            CompareError::CalculationFailed(
                "First histogram contained empty bins - relative error calculation impossible!"
                    .to_owned(),
            )
        })?,
        Metric::Intersection => intersection(&first_hist, &second_hist),
        Metric::Hellinger => hellinger(&first_hist, &second_hist).ok_or_else(|| {
            CompareError::CalculationFailed(
                "One or both histograms were not normalizable!".to_owned(),
            )
        })?,
    };
    Ok(score)
}

struct Histogram {
    data: Vec<f64>,
}

impl Histogram {
    pub fn get_bin_content(&self, bin: u8) -> f64 {
        self.data[bin as usize]
    }

    pub fn from_gray_image(image: &GrayImage) -> Histogram {
        let mut data = vec![0.; 256];
        image.pixels().for_each(|p| data[p[0] as usize] += 1.);
        Histogram { data }
    }

    pub fn mean(&self) -> f64 {
        self.data.iter().sum::<f64>() / self.data.len() as f64
    }

    pub fn variance(&self) -> f64 {
        let mean = self.mean();
        self.data.iter().map(|v| (v - mean).powi(2)).sum()
    }

    pub fn integral(&self) -> f64 {
        self.data.iter().sum()
    }

    #[cfg(test)]
    pub fn from_vec(data: Vec<f64>) -> Option<Histogram> {
        if data.len() != 256 {
            None
        } else {
            Some(Histogram { data })
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn correlation_simple() {
        let mut first_vec = vec![255.; 256];
        first_vec[0] = 0.;
        let mut second_vec = vec![0.; 256];
        second_vec[0] = 255.;

        let first = Histogram::from_vec(first_vec).unwrap();
        let second = Histogram::from_vec(second_vec).unwrap();

        assert_eq!(correlation(&first, &first).unwrap(), 1.);
        assert_eq!(correlation(&first, &second).unwrap(), -1.);
    }

    #[test]
    fn correlation_zero_variance() {
        let first_vec = vec![0.; 256];

        let first = Histogram::from_vec(first_vec).unwrap();

        assert!(correlation(&first, &first).is_none());
    }

    #[test]
    fn chi_square_simple() {
        let first_vec = vec![10.; 256];
        let second_vec = vec![0.; 256];

        let first = Histogram::from_vec(first_vec).unwrap();
        let second = Histogram::from_vec(second_vec).unwrap();

        assert_eq!(chi_square(&first, &first).unwrap(), 0.);
        assert_eq!(chi_square(&first, &second).unwrap(), 10. * 256.);
    }

    #[test]
    fn chi_square_edge_cases() {
        let first_vec = vec![0.; 256];
        let second_vec = vec![10.; 256];

        let first = Histogram::from_vec(first_vec).unwrap();
        let second = Histogram::from_vec(second_vec).unwrap();

        assert_eq!(chi_square(&first, &first).unwrap(), 0.);
        assert!(chi_square(&first, &second).is_none());
    }

    #[test]
    fn intersection_simple() {
        let first_vec = vec![1.; 256];
        let second_vec = vec![10.; 256];

        let first = Histogram::from_vec(first_vec).unwrap();
        let second = Histogram::from_vec(second_vec).unwrap();

        assert_eq!(intersection(&first, &first), first.integral());
        assert_eq!(intersection(&first, &second), first.integral());
    }

    #[test]
    fn hellinger_tests() {
        let first_vec = vec![1.; 256];
        let second_vec = vec![10.; 256];
        let mut third_vec = vec![0.; 256];
        for i in 0..127 {
            third_vec[i] = 100.;
        }

        let zeros = vec![0.; 256];
        let zeros = Histogram::from_vec(zeros).unwrap();

        let first = Histogram::from_vec(first_vec).unwrap();
        let second = Histogram::from_vec(second_vec).unwrap();
        let third = Histogram::from_vec(third_vec).unwrap();

        assert_eq!(hellinger(&first, &first).unwrap(), 0.0);
        assert_eq!(hellinger(&first, &second).unwrap(), 5.1619136559035694e-8);
        assert_eq!(hellinger(&first, &third).unwrap(), 0.5437469730039513);
        assert!(hellinger(&first, &zeros).is_none());
    }
}
